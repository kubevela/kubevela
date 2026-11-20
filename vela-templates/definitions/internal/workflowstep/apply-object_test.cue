import "vela/test"

// An object that names no namespace is applied into default, as the kube
// provider defaults it, not into the step's namespace.
"applies the object to the cluster": test.#WorkflowStepExec & {
	definition: "apply-object"
	parameter: value: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "applied", data: tier: "gold"}
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: {cluster: "", value: metadata: name: "applied"}}]
		resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "applied", namespace: "default"}, data: tier: "gold"}]
	}
}

"updates an object that already exists": test.#WorkflowStepExec & {
	definition: "apply-object"
	parameter: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "updated", namespace: "shop"}, data: tier: "gold"}
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "updated", namespace: "shop"}, data: {tier: "silver", region: "eu"}}]
	expect: {
		phase: "succeeded"
		resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "updated", namespace: "shop"}, data: tier: "gold"}]
	}
}

"passes the cluster through": test.#WorkflowStepExec & {
	definition: "apply-object"
	parameter: {value: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "elsewhere"}, cluster: "eu-1"}
	mocks: "vela/kube": "#Apply": $returns: value: {}
	expect: calls: "vela/kube": "#Apply": [{$params: cluster: "eu-1"}]
}

"an object the API server rejects fails the step": test.#WorkflowStepExec & {
	definition: "apply-object"
	parameter: value: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "Not_A_Valid_Name"}
	expect: {
		phase:   "failed"
		message: =~"Not_A_Valid_Name"
	}
}
