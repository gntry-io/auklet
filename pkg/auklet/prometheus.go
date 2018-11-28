package auklet

import (
	"fmt"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"net/url"
	"strconv"
	"time"
	"context"
	"errors"
)

// construct a new Prometheus API object to use for querying
func NewPrometheusAPI(endpoint string) (api.Client, error) {
	// validate endpoint url
	uri, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error validating Prometheus endpoint: %v", err)
	}

	// get client
	client, err := api.NewClient(api.Config{Address: uri.String()})
	if err != nil {
		return nil, fmt.Errorf("error creating Prometheus client: %v", err)
	}

	return client, nil
}

// private function to execute a query against the Prometheus endpoint, and
// return the metric
func (a *Auklet) getMetric(ctx context.Context, query string) (float64, error) {
	result := 0.0
	pc := v1.NewAPI(*a.PrometheusClient)

	value, err := pc.Query(ctx, query, time.Now())
	if err != nil {
		return result, fmt.Errorf("error executing Prometheus query: %v", err)
	}

	switch value.Type() {
	case model.ValVector:
		result, err = strconv.ParseFloat(value.(model.Vector)[0].Value.String(), 64)
		if err != nil {
			return result, fmt.Errorf("could not get value: %v", err)
		}
	default:
		return result, errors.New("query returned multiple or wrong type of value(s)")
	}

	return result, nil
}