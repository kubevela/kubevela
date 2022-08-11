lifecycle: {
	type: "trait"
	annotations: {}
	description: "Add lifecycle hooks for every container of K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	patch: spec: template: spec: containers: [...{
		lifecycle: {
			if parameter.postStart != _|_ {
				postStart: parameter.postStart
			}
			if parameter.preStop != _|_ {
				preStop: parameter.preStop
			}
		}
	}]
	parameter: {
		postStart?: #LifeCycleHandler
		preStop?:   #LifeCycleHandler
	}
	#Port: int & >=1 & <=65535
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
