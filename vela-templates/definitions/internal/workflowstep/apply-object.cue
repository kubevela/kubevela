import (
	"vela/kube"
)

"apply-object": {
	type: "workflow-step"
	annotations: {
		"category": "Resource Management"
	}
	labels: {}
	description: "Apply raw kubernetes objects for your workflow steps"
}
template: {
	apply: kube.#Apply & {
		$params: parameter
	}

	parameter: {
		// +usage=Specify Kubernetes native resource object to be applied
		value: {...}
		// +usage=The cluster you want to apply the resource to, default is the current control plane cluster
		cluster: *"" | string
	}
}
