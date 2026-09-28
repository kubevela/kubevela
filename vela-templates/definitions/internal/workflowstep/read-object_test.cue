import "vela/test"

_app: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {name: "shop", namespace: "default"}
	spec: components: [{name: "web", type: "webservice", properties: image: "nginx"}]
}

// With only a name, it reads an Application in default, not the step's
// namespace.
"reads an Application from default by default": test.#WorkflowStepExec & {
	definition: "read-object"
	parameter: name: "shop"
	resources: [_app]
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Read": [{
			$params: {
				cluster: ""
				value: {apiVersion: "core.oam.dev/v1beta1", kind: "Application", metadata: {name: "shop", namespace: "default"}}
			}
			$returns: value: spec: components: [{name: "web", type: "webservice"}]
		}]
	}
}

"reads any kind from the given namespace": test.#WorkflowStepExec & {
	definition: "read-object"
	parameter: {apiVersion: "v1", kind: "ConfigMap", name: "settings", namespace: "read-object"}
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "settings", namespace: "read-object"}, data: tier: "gold"}]
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Read": [{
			$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "settings", namespace: "read-object"}}
			$returns: value: data: tier: "gold"
		}]
	}
}

"passes the cluster through": test.#WorkflowStepExec & {
	definition: "read-object"
	parameter: {name: "shop", cluster: "eu-1"}
	mocks: "vela/kube": "#Read": $returns: value: {}
	expect: calls: "vela/kube": "#Read": [{$params: cluster: "eu-1"}]
}

// kube.#Read reports a missing object in its returns rather than failing, and
// the step passes that on in its output.
"an object that does not exist is reported, not failed": test.#WorkflowStepExec & {
	definition: "read-object"
	parameter: name: "absent"
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Read": [{$returns: err: =~"not found"}]
	}
}

"a name is required": test.#WorkflowStepExec & {
	definition: "read-object"
	expect: {
		phase:  "failed"
		reason: "Execute"
	}
}
