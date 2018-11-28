package auklet

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Auklet contains global state
type Auklet struct {
	sync.Mutex
	DockerClient     *client.Client
	PrometheusClient *api.Client
	CancelMonitor    map[string]func()
	HTTPServer       *http.Server
	metrics          map[string]prometheus.Metric
	serviceMetrics   map[string]map[string]prometheus.Metric
}

// New initializes a new Auklet instance for us; it validates required
// parameters, and converts them to more usable types if necessary.
func New(promURL string, port int) (*Auklet, error) {

	dockerClient, err := client.NewEnvClient()
	defer dockerClient.Close()

	if err != nil {
		log.Error("Auklet aborted flight")
		return nil, fmt.Errorf("error creating Docker client: %v", err)
	}

	var pURL *url.URL
	if promURL == "" {
		return nil, errors.New("prometheus url must be provided")
	}

	pURL, err = url.Parse(promURL)
	if err != nil {
		return nil, fmt.Errorf("invalid prometheus url: %v", err)
	}

	promClient, err := NewPrometheusAPI(pURL.String())
	if err != nil {
		return nil, fmt.Errorf("error creating Prometheus client: %v", err)
	}

	return &Auklet{
		DockerClient:     dockerClient,
		PrometheusClient: &promClient,
		CancelMonitor:    make(map[string]func()),
		HTTPServer:       NewWebServer(port),
		metrics:          registerGlobalMetrics(),
		serviceMetrics:    make(map[string]map[string]prometheus.Metric),
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
		return err
	}

	for _, s := range services {
		a.startMonitor(ctx, s)
	}

	errorChan := make(chan error, 1)
	go a.receiveDockerEvents(ctx, errorChan)
	go a.startWebServer(ctx)

	select {
	case s := <-interrupt:
		log.WithFields(log.Fields{"signal": s}).Info("Received OS signal")
		cancel()
		_ = a.HTTPServer.Shutdown(ctx)
		// Give goroutines some time to finish and clean up
		time.Sleep(2 * time.Second)

	case e := <-errorChan:
		log.Error(e)
		log.Info("Shutting down")
		cancel()
		_ = a.HTTPServer.Shutdown(ctx)
		// Give goroutines some time to finish and clean up
		time.Sleep(2 * time.Second)
	}

	log.Info("Auklet landed")
	return nil
}

// NewWebServer returns a new HTTP server configured for serving all Auklet
// endpoints.
func NewWebServer(port int) *http.Server {
	// Create a new router
	router := mux.NewRouter()

	// Register pprof handlers
	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	router.Handle("/debug/pprof/block", pprof.Handler("block"))

	router.Handle("/metrics", promhttp.Handler())

	// By storing the HTTP Server, we are able to Shutdown gracefully..
	return &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
}

// startWebServer takes a context and starts the webserver
func (a *Auklet) startWebServer(ctx context.Context) {
	log.WithField("listen_address", a.HTTPServer.Addr).Info("HTTP endpoints activated")
	// Start serving, just log the error if server exits..
	err := a.HTTPServer.ListenAndServe()
	if err != http.ErrServerClosed {
		log.WithError(err).Error("HTTP Server stopped unexpectedly")
		_ = a.HTTPServer.Shutdown(ctx)
	} else {
		log.WithField("message", err).Info("HTTP Server stopped")
	}
}