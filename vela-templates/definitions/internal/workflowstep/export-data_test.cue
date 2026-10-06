import "vela/test"

// Placements come from the Application's topology policies, so they are
// mocked; a legacy vela/op mock's returns land beside its parameters.
_placements: {
	_placed: *[{cluster: "local", namespace: ""}] | [...]
	mocks: "vela/op": "#GetPlacementsFromTopologyPolicies": $returns: placements: _placed
}

"exports the data to a ConfigMap named after the Application": test.#WorkflowStepExec & _placements & {
	definition: "export-data"
	context: appName: "shop"
	parameter: data: tier: "gold"
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: {cluster: "local", value: {kind: "ConfigMap", metadata: name: "shop", data: tier: "gold"}}}]
		resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "shop", data: tier: "gold"}]
	}
}

"exports a Secret with the given name and namespace": test.#WorkflowStepExec & _placements & {
	definition: "export-data"
	parameter: {name: "creds", namespace: "export-data-target", kind: "Secret", data: user: "admin"}
	resources: [{apiVersion: "v1", kind: "Namespace", metadata: name: "export-data-target"}]
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: value: {kind: "Secret", stringData: user: "admin", data?: _|_}}]
		resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "creds", namespace: "export-data-target"}, data: user: "YWRtaW4="}]
	}
}

"exports to every cluster the topology places it on": test.#WorkflowStepExec & _placements & {
	definition: "export-data"
	parameter: {data: tier: "gold", topology: "topology-multi"}
	_placed: [{cluster: "eu-1", namespace: ""}, {cluster: "us-1", namespace: ""}]
	mocks: "vela/kube": "#Apply": $returns: value: {}
	expect: {
		phase: "succeeded"
		calls: "vela/kube": {
			"#Apply": [{$params: cluster: "eu-1"}, {$params: cluster: "us-1"}] @contains()
		}
	}
}

"an unknown kind is rejected": test.#WorkflowStepExec & {
	definition: "export-data"
	parameter: {kind: "Deployment", data: {}}
	mocks: "vela/op": "#GetPlacementsFromTopologyPolicies": {}
	expect: {phase: "failed", message: =~"parameter.kind"}
}
