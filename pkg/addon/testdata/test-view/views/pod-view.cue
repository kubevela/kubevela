import (
	"vela/ql"
)

parameter: {
	name:      string
	namespace: string
	cluster:   *"" | string
}
pod: ql.#Read & {
	value: {
		apiVersion: "v1"
		kind:       "Pod"
		metadata: {
			name:      parameter.name
			namespace: parameter.namespace
		}
	}
	cluster: parameter.cluster
}
eventList: ql.#SearchEvents & {
	value: {
		apiVersion: "v1"
		kind:       "Pod"
		metadata:   pod.value.metadata
	}
	cluster: parameter.cluster
}
podMetrics: ql.#Read & {
	cluster: parameter.cluster
	value: {
		apiVersion: "metrics.k8s.io/v1beta1"
		kind:       "PodMetrics"
		metadata: {
			name:      parameter.name
			namespace: parameter.namespace
		}
	}
}
status: {
	if pod.err == _|_ {
		containers: [ for container in pod.value.spec.containers {
			name:  container.name
			image: container.image
			resources: {
				if container.resources.limits != _|_ {
					limits: container.resources.limits
				}
				if container.resources.requests != _|_ {
					requests: container.resources.requests
				}
				if podMetrics.err == _|_ {
					usage: {for containerUsage in podMetrics.value.containers {
						if containerUsage.name == container.name {
							cpu:    containerUsage.usage.cpu
							memory: containerUsage.usage.memory
						}
					}}
				}
			}
			if pod.value.status.containerStatuses != _|_ {
				status: {for containerStatus in pod.value.status.containerStatuses if containerStatus.name == container.name {
					state:        containerStatus.state
					restartCount: containerStatus.restartCount
				}}
			}
		}]
		if eventList.err == _|_ {
			events: eventList.list
		}
	}
	if pod.err != _|_ {
		error: pod.err
	}
}
