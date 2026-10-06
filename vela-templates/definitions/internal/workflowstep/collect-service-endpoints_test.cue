import "vela/test"

// Collecting reads the Application's resource trees through the controller's
// query provider, which a step test cannot run, so it is always mocked:
// _found is what it finds.
_collected: {
	_found: [...{...}]
	mocks: "vela/query": "#CollectServiceEndpoints": $returns: list: _found
}

_inner: {
	endpoint: {protocol: "TCP", host: "web.prod", port: 80, portName: "http", inner: true}
	ref: {apiVersion: "v1", kind: "Service", name: "web"}
	component: "web"
}

_metrics: {
	endpoint: {protocol: "TCP", host: "web.prod", port: 9090, portName: "metrics", inner: true}
	ref: {apiVersion: "v1", kind: "Service", name: "web"}
	component: "web"
}

_public: {
	endpoint: {protocol: "TCP", host: "web.example.com", port: 443, portName: "https"}
	ref: {apiVersion: "networking.k8s.io/v1", kind: "Ingress", name: "web"}
	component: "web"
}

"collects the endpoints of the application running the step": test.#WorkflowStepExec & _collected & {
	_found: [_inner]
	definition: "collect-service-endpoints"
	context: namespace: "prod"
	expect: {
		phase: "succeeded"
		calls: "vela/query": "#CollectServiceEndpoints": [{$params: app: {
			name:      "test-app"
			namespace: "prod"
			filter: {} @exact()
		}}]
	}
}

"collects the endpoints of the given application and components": test.#WorkflowStepExec & _collected & {
	_found: [_inner]
	definition: "collect-service-endpoints"
	parameter: {name: "shop", namespace: "retail", components: ["web", "api"]}
	expect: calls: "vela/query": "#CollectServiceEndpoints": [{$params: app: {
		name:      "shop"
		namespace: "retail"
		filter: components: ["web", "api"]
	}}]
}

"waits while the application has no endpoint": test.#WorkflowStepExec & _collected & {
	_found: []
	definition: "collect-service-endpoints"
	expect: phase: "running"
}

"waits while no endpoint has the port name": test.#WorkflowStepExec & _collected & {
	_found: [_inner, _public]
	definition: "collect-service-endpoints"
	parameter: portName: "metrics"
	expect: phase:       "running"
}

"succeeds once an endpoint has the port name": test.#WorkflowStepExec & _collected & {
	_found: [_inner, _metrics]
	definition: "collect-service-endpoints"
	parameter: portName: "metrics"
	expect: phase:       "succeeded"
}

"waits while no endpoint has the port": test.#WorkflowStepExec & _collected & {
	_found: [_inner, _public]
	definition: "collect-service-endpoints"
	parameter: port: 9090
	expect: phase:   "running"
}

"succeeds once an endpoint has the port": test.#WorkflowStepExec & _collected & {
	_found: [_inner, _metrics]
	definition: "collect-service-endpoints"
	parameter: port: 9090
	expect: phase:   "succeeded"
}

"waits while an endpoint matches the port name but not the port": test.#WorkflowStepExec & _collected & {
	_found: [_inner, _metrics]
	definition: "collect-service-endpoints"
	parameter: {portName: "http", port: 9090}
	expect: phase: "running"
}

"waits while every endpoint is inner when only outer ones are wanted": test.#WorkflowStepExec & _collected & {
	_found: [_inner, _metrics]
	definition: "collect-service-endpoints"
	parameter: outer: true
	expect: phase:    "running"
}

// An endpoint that does not say whether it is inner counts as outer.
"succeeds once an outer endpoint is found": test.#WorkflowStepExec & _collected & {
	_found: [_inner, _public]
	definition: "collect-service-endpoints"
	parameter: outer: true
	expect: phase:    "succeeded"
}

"takes inner endpoints too when outer is false": test.#WorkflowStepExec & _collected & {
	_found: [_inner]
	definition: "collect-service-endpoints"
	parameter: outer: false
	expect: phase:    "succeeded"
}

"only http and https are protocols": test.#WorkflowStepExec & _collected & {
	_found: [_inner]
	definition: "collect-service-endpoints"
	parameter: protocal: "ftp"
	expect: {phase: "failed", message: =~"parameter.protocal"}
}
