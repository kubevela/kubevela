import (
	"vela/op"
	"vela/kube"
)

"counts-policies": {
	type:        "workflow-step"
	description: "Records how many policies the Application has, through a legacy vela/op function"
}
template: {
	load: op.#LoadPolicies & {}
	record: kube.#Apply & {
		$params: value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: "policy-count", namespace: context.namespace}
			data: count: "\(len(load.value))"
		}
	}
	parameter: {}
}
