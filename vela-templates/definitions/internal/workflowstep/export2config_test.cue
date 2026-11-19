import (
	"vela/test"
	"vela/kube"
)

tidy: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "exported", namespace: "export2config-target"}}} @afterEach()

"exports the data to a ConfigMap in the step's namespace": test.#WorkflowStepExec & {
	definition: "export2config"
	parameter: {configName: "exported", data: {host: "db.local", port: "5432"}}
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: {cluster: "", value: {kind: "ConfigMap", metadata: name: "exported"}}}]
		resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "exported", data: {host: "db.local", port: "5432"} @exact()}]
	}
}

"exports into the given namespace": test.#WorkflowStepExec & {
	definition: "export2config"
	parameter: {configName: "exported", namespace: "export2config-target", data: tier: "gold"}
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "marker", namespace: "export2config-target"}}]
	expect: {
		phase: "succeeded"
		resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "exported", namespace: "export2config-target"}, data: tier: "gold"}]
	}
}

"updates a ConfigMap that exists, keeping keys it does not set": test.#WorkflowStepExec & {
	definition: "export2config"
	parameter: {configName: "exported", data: tier: "gold"}
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "exported", data: {tier: "silver", keep: "me"}}]
	expect: resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "exported", data: {tier: "gold", keep: "me"} @exact()}]
}

"passes the cluster through": test.#WorkflowStepExec & {
	definition: "export2config"
	parameter: {configName: "exported", data: tier: "gold", cluster: "eu-1"}
	mocks: "vela/kube": "#Apply": $returns: value: {}
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: cluster: "eu-1"}]
	}
}

"the ConfigMap name is required": test.#WorkflowStepExec & {
	definition: "export2config"
	parameter: data: tier: "gold"
	expect: {
		phase:   "failed"
		message: =~"parameter.configName"
	}
}
