import "vela/kube"

reader: {
	type: "component"
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
}
template: {
	secret: kube.#Get & {
		$params: resource: {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {name: parameter.secret, namespace: context.namespace}
		}
	}
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		data: password: secret.$returns.data.password
	}
	parameter: secret: string
}
