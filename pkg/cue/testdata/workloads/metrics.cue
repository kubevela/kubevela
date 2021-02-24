#metrics: {
	// +usage=format of the metrics, default as prometheus
	// +short=f
	format:  *"prometheus" | string
	enabled: *true | bool
	port?:   *8080 | >=1024 & <=65535 & int
	// +usage=the label selector for the pods, default is the workload labels
	selector?: [string]: string
}
outputs: metrics: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "MetricsTrait"
	spec: {
		scrapeService: parameter
	}
}
parameter: #metrics
