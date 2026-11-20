import (
	"vela/kube"
	"vela/http"
)

notify: {
	type:        "workflow-step"
	description: "Posts a ConfigMap's message to a webhook"
}
template: {
	settings: kube.#Read & {
		$params: value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: parameter.config, namespace: context.namespace}
		}
	}
	post: http.#HTTPDo & {
		$params: {
			method: "POST"
			url:    parameter.webhook
			request: body: settings.$returns.value.data.message
		}
	}
	parameter: {
		config:  string
		webhook: string
	}
}
