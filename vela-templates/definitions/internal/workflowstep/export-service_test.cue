import "vela/test"

// Placements come from the Application's topology policies, so they are
// mocked; a legacy vela/op mock's returns land beside its parameters.
_placements: {
	_placed: *[{cluster: "local", namespace: ""}] | [...]
	mocks: "vela/op": "#GetPlacementsFromTopologyPolicies": $returns: placements: _placed
}

_endpoint: {ip: "10.0.0.8", port: 80, targetPort: 8080}

"exports a Service and its Endpoints named after the Application": test.#WorkflowStepExec & _placements & {
	definition: "export-service"
	context: appName: "legacy-db"
	parameter: _endpoint
	expect: {
		phase: "succeeded"
		resources: [{
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: "legacy-db"
			spec: {type: "ClusterIP", ports: [{protocol: "TCP", port: 80, targetPort: 8080}]}
		}, {
			apiVersion: "v1"
			kind:       "Endpoints"
			metadata: name: "legacy-db"
			subsets: [{addresses: [{ip: "10.0.0.8"}], ports: [{port: 8080}]}]
		}]
	}
}

"exports with the given name and namespace": test.#WorkflowStepExec & _placements & {
	definition: "export-service"
	parameter: _endpoint & {name: "db", namespace: "export-service-target"}
	resources: [{apiVersion: "v1", kind: "Namespace", metadata: name: "export-service-target"}]
	expect: resources: [
		{apiVersion: "v1", kind: "Service", metadata: {name: "db", namespace: "export-service-target"}},
		{apiVersion: "v1", kind: "Endpoints", metadata: {name: "db", namespace: "export-service-target"}},
	]
}

"exports to every cluster the topology places it on": test.#WorkflowStepExec & _placements & {
	definition: "export-service"
	parameter: _endpoint & {topology: "topology-multi"}
	_placed: [{cluster: "eu-1", namespace: ""}, {cluster: "us-1", namespace: ""}]
	mocks: "vela/kube": "#Apply": $returns: value: {}
	expect: calls: "vela/kube": {
		"#Apply": [
			{$params: {cluster: "eu-1", value: kind: "Service"}},
			{$params: {cluster: "eu-1", value: kind: "Endpoints"}},
			{$params: {cluster: "us-1", value: kind: "Service"}},
			{$params: {cluster: "us-1", value: kind: "Endpoints"}},
		] @contains()
	}
}

"a port that is not a number is rejected": test.#WorkflowStepExec & {
	definition: "export-service"
	parameter: {ip: "10.0.0.8", port: "http", targetPort: 8080}
	mocks: "vela/op": "#GetPlacementsFromTopologyPolicies": {}
	expect: {phase: "failed", message: =~"parameter.port"}
}
