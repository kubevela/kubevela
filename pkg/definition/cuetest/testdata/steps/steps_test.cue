import "vela/test"

"publishes the ConfigMap": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "cfg", data: a: "1"}
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: value: kind: "ConfigMap"}]
		resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "cfg", data: a: "1"}]
	}
}

"into the step's namespace": test.#WorkflowStepExec & {
	definition: "publish"
	context: namespace: "shop"
	parameter: {name: "cfg", data: {}}
	expect: resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "cfg", namespace: "shop"}}]
}

_settings: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: message: "hello"}
_notify: {
	definition: "notify"
	parameter: {config: "settings", webhook: "https://hooks.example.com/deploy"}
	resources: [_settings]
}

"reads for real, posts to a mock": test.#WorkflowStepExec & _notify & {
	mocks: "vela/http": "#HTTPDo": $returns: {statusCode: 202, body: "ok"}
	expect: {
		phase: "succeeded"
		calls: {
			"vela/kube": "#Read":   [{$returns: value: data: message: "hello"}]
			"vela/http": "#HTTPDo": [{$params: {method: "POST", request: body: "hello"}}]
		}
	}
}

"an unmocked post fails the step": test.#WorkflowStepExec & _notify & {
	expect: {
		phase:   "failed"
		message: =~"reaches outside the cluster"
	}
}

_deployment: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "web"
	spec: {
		replicas: 1
		selector: matchLabels: app: "web"
		template: {
			metadata: labels: app: "web"
			spec: containers: [{name: "web", image: "nginx"}]
		}
	}
}

"waits while the Deployment is not ready": test.#WorkflowStepExec & {
	definition: "wait-ready"
	parameter: name: "web"
	resources: [_deployment & {status: {replicas: 1, readyReplicas: 0}}]
	expect: {
		phase:   "running"
		message: "waiting for web"
	}
}

"done once it is": test.#WorkflowStepExec & {
	definition: "wait-ready"
	parameter: name: "web"
	resources: [_deployment & {status: {replicas: 1, readyReplicas: 1}}]
	expect: phase: "succeeded"
}
