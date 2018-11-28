package auklet

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	log "github.com/sirupsen/logrus"
	"time"
)

// receiveDockerEvents listens to the Docker event stream to see if any services
// are added or updated in the Swarm, and update the internal administration
// (labels) accordingly.
func (a *Auklet) receiveDockerEvents(ctx context.Context, errorChan chan error) {
	// Services only emit create, update and remove events
	eventsFilter := filters.NewArgs()
	eventsFilter.Add("type", "service")

	dctx, cancel := context.WithCancel(ctx)
	eventChan, errChan := a.DockerClient.Events(dctx, types.EventsOptions{
		Filters: eventsFilter,
		Since: time.Now().Format(time.RFC3339Nano),
	})
	log.Info("Docker event listener started")

	for {
		select {
		case e := <-eventChan:
			serviceName := e.Actor.Attributes["name"]
			serviceID := e.Actor.ID

			eventLogger := log.WithFields(log.Fields{
				"event":        e.Action,
				"service_name": serviceName,
				"service_id":   serviceID,
				"event_type":   e.Type,
			})

			eventLogger.Info("Docker service event received")

			switch e.Action {
			case "remove":
				eventLogger.Debug("Delete monitor")
				a.deleteMonitor(serviceID)

			case "create":
				eventLogger.Debug("Add monitor")
				a.addMonitor(ctx, serviceID)

			// TODO: prevent recreating the monitor due to updates
			case "update":
				// Delete and re-create monitor
				eventLogger.Debug("Re-create monitor")
				a.deleteMonitor(serviceID)
				a.addMonitor(ctx, serviceID)
			}

		case ee := <-errChan:
			errorChan <- fmt.Errorf("error connecting to Docker: %v", ee)

		case <-ctx.Done():
			log.Info("Stopping Docker event listener")
			cancel()
			return
		}
	}
}

// private function that retrieves service information from Docker Swarm API
// using the service ID.
func (a *Auklet) getServiceByID(ctx context.Context, id string) (*swarm.Service, error) {
	log.WithField("service_id", id).Debug("Query service from Docker Swarm")

	serviceFilter := filters.NewArgs()
	serviceFilter.Add("id", id)
	services, err := a.DockerClient.ServiceList(ctx, types.ServiceListOptions{Filters: serviceFilter})
	if err != nil {
		return nil, fmt.Errorf("could not query service from Docker Swarm: %v", err)
	}

	log.WithField("service_count", len(services)).Debug("service query returned")
	if len(services) != 1 {
		return nil, errors.New("could not reliably get service from Docker Swarm")
	}

	return &services[0], nil
}

// getServices fetches a list of all services currently running on the Swarm
// and populates our map with services and their configuration from the
// service labels.
func (a *Auklet) getAllServices(ctx context.Context) ([]swarm.Service, error) {
	log.Info("Fetching service list from Docker Swarm")
	services, err := a.DockerClient.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not fetch services from Docker Swarm: %v", err)
	}

	if len(services) == 0 {
		log.Info("No services found on Docker Swarm")
	}

	for _, s := range services {
		log.WithFields(log.Fields{
			"name":     s.Spec.Name,
			"id":       s.ID,
			"labels":   s.Spec.Labels,
			"replicas": *s.Spec.Mode.Replicated.Replicas,
		}).Info("Found service")

	}
	return services, nil
}

// Private function that actually updates the service and sets the required
// replicas.
func (a *Auklet) scaleService(serviceID string, replicas int) {
	service, _, err := a.DockerClient.ServiceInspectWithRaw(context.Background(), serviceID)
	if err != nil {
		log.Error(err)
	}

	serviceMode := &service.Spec.Mode
	if serviceMode.Replicated == nil {
		log.Error("can't scale: service is not replicated mode")
	}
	currentReplicas := int(*service.Spec.Mode.Replicated.Replicas)
	r := uint64(replicas)
	serviceMode.Replicated.Replicas = &r

	// Only perform scaling if the service is in a stable/completed state to
	// prevent race conditions.
	if service.UpdateStatus.State == swarm.UpdateStateCompleted || a.serviceReady(context.Background(), service.ID, currentReplicas) {
		response, err := a.DockerClient.ServiceUpdate(context.Background(), service.ID, service.Version, service.Spec, types.ServiceUpdateOptions{})
		if err != nil {
			log.Error(err)
		}

		for _, warning := range response.Warnings {
			log.Warnf("response: %s", warning)
		}

		log.WithFields(log.Fields{
			"service_id": serviceID,
			"replicas":   r,
		}).Info("scaled service")

	} else {
		log.WithFields(log.Fields{
			"service_id": serviceID,
			"state":      service.UpdateStatus.State,
			"msg":        service.UpdateStatus.Message,
		}).Info("wait: service not ready to scale")
	}
}

// function to get all *ready* tasks within the service
func (a *Auklet) getReadyServiceTasks(ctx context.Context, serviceID string) ([]swarm.Task, error) {
	taskFilter := filters.NewArgs()
	taskFilter.Add("service", serviceID)
	taskFilter.Add("_up-to-date", "true")

	return a.DockerClient.TaskList(ctx, types.TaskListOptions{Filters: taskFilter})
}

// serviceReady counts all ready tasks within a service, and if this equals
// to the number of replicas the service is probably ready.
func (a *Auklet) serviceReady(ctx context.Context, serviceID string, replicas int) bool {
	serviceLog := log.WithFields(log.Fields{
		"service_id": serviceID,
		"replicas": replicas,
	})

	tasks, err := a.getReadyServiceTasks(ctx, serviceID)
	if err != nil {
		serviceLog.Error("error while querying service ready tasks: %v", err)
		return false
	}

	serviceLog.Debugf("Found %d ready tasks for service", len(tasks))

	if len(tasks) != replicas {
		log.Debug("Service not in ready state")
		return false
	}
	serviceLog.Debug("Service in ready state")
	return true
}
