import (
	"encoding/base64"
	"encoding/json"
	"vela/test"
)

_accepted: mocks: "vela/http": "#HTTPDo": $returns: {statusCode: 202, body: ""}

"posts the given data as JSON": test.#WorkflowStepExec & _accepted & {
	definition: "webhook"
	parameter: {
		url: value: "https://hooks.example.com/deploy"
		data: {app: "shop", status: "deployed"}
	}
	expect: {
		phase: "succeeded"
		calls: {
			"vela/http": "#HTTPDo": [{$params: {
				method: "POST"
				url:    "https://hooks.example.com/deploy"
				request: {
					body: json.Marshal({app: "shop", status: "deployed"})
					header: "Content-Type": "application/json"
				} @exact()
			}}]
			"vela/kube"?: _|_
		}
	}
}

_app: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: name: "shop"
	spec: components: [{name: "web", type: "webservice", properties: image: "nginx"}]
}

"posts the Application when no data is given": test.#WorkflowStepExec & _accepted & {
	definition: "webhook"
	context: appName: "shop"
	parameter: url: value: "https://hooks.example.com/deploy"
	resources: [_app]
	expect: {
		phase: "succeeded"
		calls: {
			"vela/kube": "#Read": [{$params: value: {apiVersion: "core.oam.dev/v1beta1", kind: "Application", metadata: name: "shop"}}]
			"vela/http": "#HTTPDo": [{$params: request: body: =~"\"spec\":\\{\"components\":\\[\\{\"name\":\"web\""}]
		}
	}
}

"reads the url from a Secret": test.#WorkflowStepExec & _accepted & {
	definition: "webhook"
	parameter: {
		url: secretRef: {name: "hooks", key: "deploy"}
		data: app: "shop"
	}
	resources: [{apiVersion: "v1", kind: "Secret", metadata: name: "hooks", data: deploy: base64.Encode(null, "https://hooks.example.com/secret")}]
	expect: {
		phase: "succeeded"
		calls: "vela/http": "#HTTPDo": [{$params: url: "https://hooks.example.com/secret"}]
	}
}

"passes the timeout through": test.#WorkflowStepExec & _accepted & {
	definition: "webhook"
	parameter: {
		url: value: "https://hooks.example.com/deploy"
		data: app:  "shop"
		timeout: "30s"
	}
	expect: calls: "vela/http": "#HTTPDo": [{$params: request: timeout: "30s"}]
}

"a url is required": test.#WorkflowStepExec & _accepted & {
	definition: "webhook"
	parameter: data: app: "shop"
	expect: {
		phase:   "failed"
		message: "url is required: set url.value or url.secretRef"
		calls: "vela/http"?: _|_
	}
}
