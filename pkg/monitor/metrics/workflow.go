package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	StepDurationGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "step_duration_ms",
		Help:        "step latency distributions.",
		ConstLabels: prometheus.Labels{},
	}, []string{"namespace", "application", "workflow_revision", "step_name", "step_type"})
)
