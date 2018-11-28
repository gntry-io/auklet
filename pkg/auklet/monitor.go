package auklet

import (
	"context"
	"github.com/docker/docker/api/types/swarm"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

// monitorService takes a Service, and starts monitoring it
func (a *Auklet) monitorService(ctx context.Context, s swarm.Service) {
	monitorLogger := log.WithFields(log.Fields{
		"service_id":   s.ID,
		"service_name": s.Spec.Name,
		"replicas":     *s.Spec.Mode.Replicated.Replicas,
	})
	monitorLogger.Debug("Monitor started")

	svc, err := getService(a, &s)
	if err != nil {
		monitorLogger.Error(err)
		// Defunct poller needs to cancel itself to prevent ctx leaks
		a.deleteMonitor(s.ID)
	} else {
		timer := time.NewTicker(svc.PollInterval)

		for {
			select {
			case <-timer.C:
				monitorLogger.Debug("Poll Prometheus")
				m, err := a.getMetric(ctx, svc.Query)
				if err != nil {
					monitorLogger.WithField("error", err).Error("Error while executing Prometheus query")
				}
				monitorLogger.Debugf("Query returned: %f", m)

				s, err := a.getServiceByID(ctx, svc.ServiceID)
				if err != nil {
					monitorLogger.WithField("error", err).Error("Error while querying service from Docker")
				}

				if s != nil {
					svc.CurrentReplicas = int(*s.Spec.Mode.Replicated.Replicas)
				}

				monitorLogger.Debugf("Current state: %s", svc.state)

				if svc.CurrentReplicas < svc.MinReplicas {
					monitorLogger.Debug("Emitting 'scale' event")
					svc.scale(svc.MinReplicas)
				} else if svc.CurrentReplicas > svc.MaxReplicas {
					monitorLogger.Debug("Emitting 'scale' event")
					svc.scale(svc.MaxReplicas)
				} else if m < svc.DownThreshold {
					monitorLogger.Debug("Emitting 'under_threshold' event")
					svc.underThreshold()
				} else if m > svc.UpThreshold {
					monitorLogger.Debug("Emitting 'over_threshold' event")
					svc.overThreshold()
				} else {
					monitorLogger.Debug("Emitting 'stable' event")
					svc.stable()
				}

			case <-ctx.Done():
				// cancel() was called
				monitorLogger.Debug("Monitor stopped")
				return
			}
		}
	}
	monitorLogger.Debug("Monitor exited")
}

// startMonitor launches a new service monitor when the service has a label
// called `auklet.autoscale` set to true.
func (a *Auklet) startMonitor(ctx context.Context, s swarm.Service) {
	if _, found := a.CancelMonitor[s.ID]; !found {
		if enable, ok := s.Spec.Labels["auklet.autoscale"]; ok {
			if e, _ := strconv.ParseBool(enable); e {
				ctx, cancel := context.WithCancel(ctx)
				a.Lock()
				a.CancelMonitor[s.ID] = cancel
				a.Unlock()
				go a.monitorService(ctx, s)
				return
			}
		}
		log.WithField("service_id", s.ID).Info("Ignore service; auklet.autoscale not set")
	} else {
		log.WithField("service_id", s.ID).Debug("Monitor already present")
	}
}

// deleteMonitor calls cancel on the monitor to stop it, and deletes
// the cancel() func from Auklet's CancelMonitor map
func (a *Auklet) deleteMonitor(serviceID string) {
	if cancel, ok := a.CancelMonitor[serviceID]; ok {
		a.Lock()
		cancel()
		delete(a.CancelMonitor, serviceID)
		a.Unlock()
	}
}

// addMonitor queries the service by its ID and starts a monitor for it
func (a *Auklet) addMonitor(ctx context.Context, serviceID string) {
	s, err := a.getServiceByID(ctx, serviceID)
	if err != nil {
		log.WithFields(log.Fields{
			"service_id": serviceID,
			"error":      err,
		}).Error("Error while querying service")
		return
	}
	a.startMonitor(ctx, *s)
}
