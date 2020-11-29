output: {
	apiVersion: "standard.oam.dev/v1alpha1"
	kind:       "MetricsTrait"
	spec: {
		scrapeService: parameter
	}
}
parameter: {
	// +usage=Format of the metrics, default as prometheus
	// +short=f
	format: *"prometheus" | string
	// +usage=The metrics path of the service
	path: *"/metrics" | string
	// +usage=The way to retrieve data which can take the values `http` or `https`
	scheme:  *"http" | string
	enabled: *true | bool
	// +usage=The port for metrics, will discovery automatically by default
	port: *0 | >=1024 & <=65535 & int
	// +usage=The label selector for the pods, will discovery automatically by default
	selector?: [string]: string
}
