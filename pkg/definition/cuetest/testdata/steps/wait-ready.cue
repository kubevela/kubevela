import (
	"vela/kube"
	"vela/builtin"
)

"wait-ready": {
	type:        "workflow-step"
	description: "Waits until a Deployment's replicas are ready"
}
template: {
	read: kube.#Read & {
		$params: value: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: {name: parameter.name, namespace: context.namespace}
		}
	}
	// The API server leaves readyReplicas out while it is zero.
	ready: *0 | int
	if read.$returns.value.status.readyReplicas != _|_ {
		ready: read.$returns.value.status.readyReplicas
	}
	wait: builtin.#ConditionalWait & {
		$params: {
			continue: ready == read.$returns.value.spec.replicas
			message:  "waiting for \(parameter.name)"
		}
	}
	parameter: name: string
}
