lifecycle: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add lifecycle hooks to workloads."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: spec: template: spec: containers: [{
		lifecycle: {
			if parameter.postStart != _|_ {
				postStart: parameter.postStart
			}
			if parameter.preStop != _|_ {
				preStop: parameter.preStop
			}
		}
	}, ...]
	parameter: {
		postStart?: #LifeCycleHandler
		preStop?:   #LifeCycleHandler
	}
	#Port:             int & >=1 & <=65535
	#LifeCycleHandler: {
		exec?: command: [...string]
		httpGet?: {
			path?:  string
			port:   #Port
			host?:  string
			scheme: *"HTTP" | "HTTPS"
			httpHeaders?: [...{
				name:  string
				value: string
			}]
		}
		tcpSocket?: {
			port:  #Port
			host?: string
		}
	}
}
