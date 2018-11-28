package auklet

import (
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/swarm"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

// Constants representing the Service states
const (
	StateStable = iota
	StateUnderThreshold
	StateOverThreshold
	StateScaling
)

// Service is the finite state machine that represents a Swarm Service within
// Auklet.
type Service struct {
	ServiceID       string
	PollInterval    time.Duration
	CurrentReplicas int
	MinReplicas     int
	MaxReplicas     int
	UpStep          int
	DownStep        int
	Query           string
	UpThreshold     float64
	DownThreshold   float64
	UpGracePeriod   time.Duration
	DownGracePeriod time.Duration
	GraceTimer      time.Time
	auklet          *Auklet
	state           int
}

// getService takes service labels and copies/normalizes/validates them
// into a Service struct
func getService(a *Auklet, s *swarm.Service) (*Service, error) {

	pollingInterval := 30 * time.Second
	if v, isSet := s.Spec.Labels["auklet.polling_interval"]; isSet {
		pollingInterval, _ = time.ParseDuration(v)
	}

	scaleMin, err := getServiceLabelIntVal(s, "auklet.scale_min")
	if err != nil {
		return &Service{}, err
	}

	scaleMax, err := getServiceLabelIntVal(s, "auklet.scale_max")
	if err != nil {
		return &Service{}, err
	}

	upStep, err := getServiceLabelIntVal(s, "auklet.up_step", 1)
	if err != nil {
		return &Service{}, err
	}

	downStep, err := getServiceLabelIntVal(s, "auklet.down_step", 1)
	if err != nil {
		return &Service{}, err
	}

	upThreshold, err := getServiceLabelFloatVal(s, "auklet.up_threshold")
	if err != nil {
		return &Service{}, err
	}

	downThreshold, err := getServiceLabelFloatVal(s, "auklet.down_threshold")
	if err != nil {
		return &Service{}, err
	}

	upGracePeriod, err := time.ParseDuration(s.Spec.Labels["auklet.up_graceperiod"])
	if err != nil {
		upGracePeriod = 0
	}

	downGracePeriod, err := time.ParseDuration(s.Spec.Labels["auklet.down_graceperiod"])
	if err != nil {
		downGracePeriod = 0
	}

	query := ""
	if v, isSet := s.Spec.Labels["auklet.query"]; isSet {
		query = v
	} else {
		return &Service{}, errors.New("auklet.query must be set")
	}

	svc := Service{
		ServiceID:       s.ID,
		PollInterval:    pollingInterval,
		MinReplicas:     scaleMin,
		MaxReplicas:     scaleMax,
		UpStep:          upStep,
		DownStep:        downStep,
		Query:           query,
		UpThreshold:     upThreshold,
		DownThreshold:   downThreshold,
		UpGracePeriod:   upGracePeriod,
		DownGracePeriod: downGracePeriod,
		auklet:          a,
		state:           StateStable,
	}

	log.Debugf("PollingInterval: %s", pollingInterval.String())
	log.Debugf("MinReplicas:     %d", scaleMin)
	log.Debugf("MaxReplicas:     %d", scaleMax)
	log.Debugf("UpStep:          %d", upStep)
	log.Debugf("downStep:        %d", downStep)
	log.Debugf("Query:           %s", query)
	log.Debugf("UpThreshold:     %f", upThreshold)
	log.Debugf("DownThreshold:   %f", downThreshold)
	log.Debugf("UpGracePeriod:   %s", upGracePeriod.String())
	log.Debugf("DownGracePeriod: %s", downGracePeriod.String())

	return &svc, nil
}

// stable is called whenever the service enters "stable" (again)
func (s *Service) stable() {
	log.Debug("Service stable")
	// reset graceperiod timer
	s.GraceTimer = time.Time{}
	s.state = StateStable
}

// overThreshold is called whenever the service's Prometheus query
// returned a metric value that is over the defined UpThreshold
func (s *Service) overThreshold() {
	log.Debug("Service over threshold")
	if s.state == StateStable || s.state == StateUnderThreshold {
		// Reset last time over threshold
		s.GraceTimer = time.Now()
	}
	s.state = StateOverThreshold
	if time.Now().Sub(s.GraceTimer) >= s.UpGracePeriod {
		r := 0
		if s.CurrentReplicas+s.UpStep <= s.MaxReplicas {
			r = s.CurrentReplicas + s.UpStep
		} else {
			r = s.MaxReplicas
		}
		s.scale(r)
	} else {
		log.Debugf("Service in grace period (%s)", time.Now().Sub(s.GraceTimer).String())
	}
}

// underThreshold is called whenever the service's Prometheus query
// returned a metric value that is under the defined DownThreshold
func (s *Service) underThreshold() {
	log.Debug("Service under threshold")
	if s.state == StateStable || s.state == StateOverThreshold {
		// Reset last time under threshold
		s.GraceTimer = time.Now()
	}
	s.state = StateUnderThreshold
	if time.Now().Sub(s.GraceTimer) >= s.DownGracePeriod {
		r := 0
		if s.CurrentReplicas-s.DownStep >= s.MinReplicas {
			r = s.CurrentReplicas - s.DownStep
		} else {
			r = s.MinReplicas
		}
		s.scale(r)
	} else {
		log.Debugf("Service in grace period (%s)", time.Now().Sub(s.GraceTimer).String())
	}
}

// scale is called whenever the service actually needs scaling
func (s *Service) scale(replicas int) {
	log.WithField("replicas", replicas).Debug("Service scaling")
	s.state = StateScaling
	if s.CurrentReplicas != replicas {
		s.auklet.scaleService(s.ServiceID, replicas)
	}
	// after scaling return to stable state
	s.stable()
}

// getServiceLabelIntVal takes the swarm service and tries to find a specific
// service label. It will then try to take the int value from it, or return the
// default value. When no default value is set, an error is returned.
func getServiceLabelIntVal(s *swarm.Service, label string, defVal ...int) (int, error) {
	var val int
	var err error
	if v, isSet := s.Spec.Labels[label]; isSet {
		val, err = strconv.Atoi(v)
		if err != nil {
			return val, fmt.Errorf("invalid value for %s: %v", label, err)
		}
	} else if len(defVal) == 0 {
		return val, fmt.Errorf("%s must be set", label)
	} else {
		val = defVal[0]
	}
	return val, nil
}

// getServiceLabelFloatVal takes the swarm service and tries to find a specific
// service label. It will then try to take the float64 value from it, or return
// the default value. If no default value specified, an error will be returned.
func getServiceLabelFloatVal(s *swarm.Service, label string, defVal ...float64) (float64, error) {
	var val float64
	var err error
	if v, isSet := s.Spec.Labels[label]; isSet {
		val, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return val, fmt.Errorf("invalid value for %s: %v", label, err)
		}
	} else if len(defVal) == 0 {
		return val, fmt.Errorf("%s must be set", label)
	} else {
		val = defVal[0]
	}
	return val, nil
}
