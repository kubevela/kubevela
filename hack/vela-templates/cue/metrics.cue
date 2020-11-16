output: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "MetricsTrait"
	spec: {
		scrapeService: parameter
	}
}
parameter: {
	// +usage=format of the metrics, default as prometheus
	// +short=f
	format: *"prometheus" | string
	// +usage= the metrics path of the service
	path:    *"/metrics" | string
	scheme:  *"http" | string
	enabled: *true | bool
	// +usage= the port for metrics, will discovery automatically by default
	port: *0 | >=1024 & <=65535 & int
	// +usage= the label selector for the pods, will discovery automatically by default
	selector?: [string]: string
}
