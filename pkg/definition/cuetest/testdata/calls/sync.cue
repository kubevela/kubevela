import (
	"vela/kube"
	"vela/http"
)

sync: {
	type: "component"
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
}
template: {
	settings: kube.#Get & {
		$params: resource: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: parameter.config, namespace: context.namespace}
		}
	}
	notify: http.#Do & {
		$params: {
			method: "POST"
			url:    parameter.webhook
			request: body: settings.$returns.data.message
		}
	}
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		data: status: "\(notify.$returns.statusCode)"
	}
	parameter: {
		config:  string
		webhook: string
	}
}
