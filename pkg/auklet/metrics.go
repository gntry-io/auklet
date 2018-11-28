package auklet

import (
	"errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Auklet specific metrics that are exposed on /metrics
const (
	MetricServicesMonitored       = "services_monitored"
	MetricPrometheusQueriesTotal  = "prometheus_queries_total"
	MetricServiceScaleEventsTotal = "scale_events_total"
	MetricScaleUpEventsCount      = "scale_up_events_count"
	MetricScaleDownEventsCount    = "scale_down_events_count"

	MetricTypeGauge = iota
	MetricTypeCounter
)

// registerMetrics uses promauto to register Auklet metrics in the prometheus
// metrics handler, and returns a map of all registered metrics.
func registerGlobalMetrics() map[string]prometheus.Metric {

	metrics := make(map[string]prometheus.Metric)

	metrics[MetricServicesMonitored] = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "auklet",
		Name:      MetricServicesMonitored,
		Help:      "Number of services currently monitored by Auklet",
	})
	metrics[MetricPrometheusQueriesTotal] = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "auklet",
		Name:      MetricPrometheusQueriesTotal,
		Help:      "Total number of Prometheus queries executed",
	})
	metrics[MetricServiceScaleEventsTotal] = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "auklet",
		Name:      MetricServiceScaleEventsTotal,
		Help:      "Total number of Prometheus queries executed",
	})

	return metrics
}

// registerServiceMetrics creates new counters for a service
func (a *Auklet) createServiceMetrics(serviceID string, serviceName string) error {
	if err := a.registerServiceMetric(serviceID, serviceName, MetricScaleUpEventsCount,
		"Number of times the service was scaled up", MetricTypeCounter); err != nil {
		return err
	}
	if err := a.registerServiceMetric(serviceID, serviceName, MetricScaleDownEventsCount,
		"Number of times the service was scaled down", MetricTypeCounter); err != nil {
		return err
	}
	return nil
}

// registerServiceMetric registers a new service specific metric if it doesn't
// already exist.
func (a *Auklet) registerServiceMetric(serviceID string, serviceName string, metricName string, desc string, metricType int) error {
	// First initialize the metrics map if it's a new service
	if _, exists := a.serviceMetrics[serviceID]; !exists {
		a.serviceMetrics[serviceID] = make(map[string]prometheus.Metric)
	}

	// If the metric doesn't exist, add it to the service metrics map
	if _, exists := a.serviceMetrics[serviceID][metricName]; !exists {
		a.Lock()
		defer a.Unlock()

		switch metricType {
		case MetricTypeCounter:
			a.serviceMetrics[serviceID][metricName] = promauto.NewCounter(prometheus.CounterOpts{
				Namespace: "auklet",
				Subsystem: "service",
				Name:      metricName,
				Help:      desc,
				ConstLabels: prometheus.Labels{
					"service":    serviceName,
					"service_id": serviceID,
				},
			})
		case MetricTypeGauge:
			a.serviceMetrics[serviceID][metricName] = promauto.NewGauge(prometheus.GaugeOpts{
				Namespace: "auklet",
				Subsystem: "service",
				Name:      metricName,
				Help:      desc,
				ConstLabels: prometheus.Labels{
					"service":    serviceName,
					"service_id": serviceID,
				},
			})
		default:
			return errors.New("unsupported or unknown metric type")
		}
	}
	return nil
}
