# Package Usage

## Context
First, this context is compatible with built-in context interface.
Also it supports fork and commit like trace span.

### Fork
`Fork` will generate a sub context that inherit the parent's tags. When new tags are added to the `sub-context`, the `parent-context` will not be affected.

### Commit
`Commit` will log the context duration, and export metrics or other execution information.

### usage
```
tracerCtx:=context.NewTraceContext(stdCtx,"$id") 
defer tracerCtx.Commit("success")

// Execute sub-code logic
subCtx:=tracerCtx.Fork("sub-id")
...
subCtx.Commit("step is executed")

```

## Metrics
First, you need register `metricVec` in package `pkg/monitor/metrics`, like below:
```
StepDurationSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:        "step_duration_ms",
		Help:        "step latency distributions.",
		Objectives:  map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		ConstLabels: prometheus.Labels{},
	}, []string{"application", "workflow_revision", "step_name", "step_type"})
```

Now, you can export metrics by context,for example
```
subCtx:=tracerCtx.Fork("sub-id",DurationMetric(func(v float64) {
					metrics.StepDurationSummary.WithLabelValues(e.app.Name, e.status.AppRevision, stepStatus.Name, stepStatus.Type).Observe(v)
				})
subCtx.Commit("export") // At this time, it will export the StepDurationSummary metrics. 			

```

Context only support `DurationMetric` exporter. you can submit pr to support more exporters.
If metrics have nothing to do with context, there is no need to extend it through context exporter
