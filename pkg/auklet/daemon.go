package auklet

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/prometheus/client_golang/api"
	log "github.com/sirupsen/logrus"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Auklet struct {
	sync.Mutex
	DockerClient     *client.Client
	PrometheusClient *api.Client
	CancelMonitor    map[string]func()
}

// New initializes a new Auklet instance for us; it validates required
// parameters, and converts them to more usable types if necessary.
func New(promURL string) (*Auklet, error) {

	dockerClient, err := client.NewEnvClient()
	defer dockerClient.Close()

	if err != nil {
		log.Error("Auklet aborted flight")
		return nil, fmt.Errorf("error creating Docker client: %v", err)
	}

	var promUrl *url.URL
	if promURL == "" {
		return nil, errors.New("prometheus url must be provided")
	} else {
		promUrl, err = url.Parse(promURL)
		if err != nil {
			return nil, fmt.Errorf("invalid prometheus url: %v", err)
		}
	}

	promClient, err := NewPrometheusAPI(promUrl.String())
	if err != nil {
		return nil, fmt.Errorf("error creating Prometheus client: %v", err)
	}

	return &Auklet{
		DockerClient:     dockerClient,
		PrometheusClient: &promClient,
		CancelMonitor:    make(map[string]func()),
	}, nil
}

// Fly actually starts the program, and waits for an OS signal/interrupt
// before cleaning up and exiting.
func (a *Auklet) Fly() error {
	log.Info("Auklet getting ready for takeoff..")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, os.Kill, syscall.SIGTERM)

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	services, err := a.getAllServices(ctx)
	if err != nil {
		// Keep retrying every 30s
		for err != nil {
			return err
		}
	}

	for _, s := range services {
		a.startMonitor(ctx, s)
	}

	errorChan := make(chan error, 1)
	go a.receiveDockerEvents(ctx, errorChan)

	select {
	case s := <-interrupt:
		log.WithFields(log.Fields{"signal": s}).Info("Received OS signal")
		cancel()
		// Give goroutines some time to finish and clean up
		time.Sleep(2 * time.Second)

	case e := <-errorChan:
		log.Error(e)
		log.Info("Shutting down")
		cancel()
		// Give goroutines some time to finish and clean up
		time.Sleep(2 * time.Second)
	}

	log.Info("Auklet landed")
	return nil
}
