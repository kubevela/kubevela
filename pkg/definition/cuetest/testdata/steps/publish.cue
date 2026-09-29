import "vela/kube"

publish: {
	type:        "workflow-step"
	description: "Publishes settings as a ConfigMap"
}
template: {
	apply: kube.#Apply & {
		$params: value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: parameter.name, namespace: context.namespace}
			data: parameter.data
		}
	}
	parameter: {
		name: string
		data: [string]: string
	}
}
