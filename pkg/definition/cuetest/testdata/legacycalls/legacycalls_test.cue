import "vela/test"

"a real legacy call records its results as returns": test.#WorkflowStepExec & {
	definition: "reads-legacy"
	parameter: name: "settings"
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: tier: "gold"}]
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#Read": [{
			$params: value: metadata: name: "settings"
			$returns: value: data: tier: "gold"
		}]
	}
}
