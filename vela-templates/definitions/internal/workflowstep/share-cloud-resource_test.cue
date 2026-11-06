import (
	"vela/kube"
	"vela/test"
)

// The step is vela/op's legacy #ShareCloudResource. What an env-binding
// Application supplies (its policies, the env's placement and patched
// components, which of them are terraform, and their health) is mocked; the
// connection Secret it reads and the copies it writes are real.
namespaces: {
	app: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "cloud-share"}}
	apps: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "cloud-share-apps"}}
} @before()

// The copies are the step's, so each case tidies them.
tidy: {
	db: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-share"}}}
	cache: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "Secret", metadata: {name: "cache", namespace: "cloud-share"}}}
	apps: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-share-apps"}}}
} @afterEach()

_db: {name: "db", type: "alibaba-rds", properties: {instance: "small", writeConnectionSecretToRef: name: "db-conn"}}
_cache: {name: "cache", type: "alibaba-redis", properties: writeConnectionSecretToRef: {}}
_web: {name: "web", type: "webservice", properties: image: "nginx"}

_connection: {apiVersion: "v1", kind: "Secret", metadata: namespace: *"cloud-share" | string, type: "Opaque", data: password: "czNjcmV0"}

_app: _mocked & {parameter: {env: *"prod" | string, policy: *"env-bindings" | string, placements: *[{cluster: "local"}] | [...]}}

_mocked: {
	_components: *[_db] | [...]
	_terraform: *[_db] | [...]
	_healthy:   *true | bool
	definition: "share-cloud-resource"
	context: {appName: "shop", namespace: "cloud-share"}
	resources: *[_connection & {metadata: name: "db-conn-prod"}] | [...]
	mocks: "vela/op": {
		"#LoadPolicies": $returns: value: "env-bindings": {type: "env-binding", properties: envs: [
			{name: "prod", placement: clusterSelector: name: "local"},
		]}
		// Only an env with a placement is answered; any other reaches the
		// provider and fails there.
		"#MakePlacementDecisions": {
			$params: inputs: placement: clusterSelector: name: "local"
			$returns: outputs: decisions: [{cluster: "local", namespace: ""}]
		}
		"#PatchApplication": $returns: outputs: spec: components: _components
		"#LoadTerraformComponents": $returns: outputs: components: _terraform
		"#GetConnectionStatus": $returns: outputs: healthy:        _healthy
	}
}

"copies the connection Secret to the Application's namespace by default": test.#WorkflowStepExec & _app & {
	expect: {
		phase: "succeeded"
		calls: "vela/op": {
			"#GetConnectionStatus": [{$params: inputs: componentName: "db-prod"}]
			"#Read": [{$params: value: metadata: {name: "db-conn-prod", namespace: "cloud-share"}}]
			"#Apply": [{$params: {cluster: "local", value: {metadata: {name: "db-conn", namespace: "cloud-share"}, type: "Opaque"}}}]
			"#ApplyComponent"?: _|_
		}
		resources: [{apiVersion: "v1", kind: "Secret", metadata: name: "db-conn", type: "Opaque", data: password: "czNjcmV0"}]
	}
}

"a placement with no cluster is the local cluster": test.#WorkflowStepExec & _app & {
	parameter: placements: [{}]
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#Apply": [{$params: cluster: "local"}]
	}
}

"copies the connection Secret to every placement": test.#WorkflowStepExec & _app & {
	parameter: placements: [{cluster: "local", namespace: "cloud-share-apps"}, {cluster: "eu-1"}]
	mocks: "vela/op": "#Apply": {$params: cluster: "eu-1", $returns: {}}
	expect: {
		phase: "succeeded"
		calls: "vela/op": {
			"#Apply": [
				{$params: {cluster: "local", value: metadata: {name: "db-conn", namespace: "cloud-share-apps"}}},
				{$params: {cluster: "eu-1", value: metadata: {name: "db-conn", namespace: "cloud-share"}}},
			] @contains()
		}
		resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-share-apps"}, data: password: "czNjcmV0"}]
	}
}

"shares nothing with no placements": test.#WorkflowStepExec & _app & {
	parameter: placements: []
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#Apply"?: _|_
	}
}

"shares every terraform component's Secret, named after the component by default": test.#WorkflowStepExec & _app & {
	_components: [_db, _cache]
	_terraform: [_db, _cache]
	resources: [_connection & {metadata: name: "db-conn-prod"}, _connection & {metadata: name: "cache-prod"}]
	expect: {
		phase: "succeeded"
		resources: [
			{apiVersion: "v1", kind: "Secret", metadata: name: "db-conn", data: password: "czNjcmV0"},
			{apiVersion: "v1", kind: "Secret", metadata: name: "cache", data: password: "czNjcmV0"},
		]
	}
}

"shares only the components that are terraform": test.#WorkflowStepExec & _app & {
	_components: [_web, _db]
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#GetConnectionStatus": [{$params: inputs: componentName: "db-prod"}]
	}
}

"does nothing when the env has no terraform components": test.#WorkflowStepExec & _app & {
	_components: [_web]
	_terraform: []
	resources: []
	expect: {
		phase: "succeeded"
		calls: "vela/op": {
			"#GetConnectionStatus"?: _|_
			"#Apply"?:               _|_
		}
	}
}

"reads the connection Secret from the namespace the component names": test.#WorkflowStepExec & _app & {
	_components: [_db & {properties: writeConnectionSecretToRef: namespace: "cloud-share-apps"}]
	_terraform: _components
	resources: [_connection & {metadata: {name: "db-conn-prod", namespace: "cloud-share-apps"}}]
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#Read": [{$params: value: metadata: {name: "db-conn-prod", namespace: "cloud-share-apps"}}]
		resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-share"}, data: password: "czNjcmV0"}]
	}
}

"uses the first env-binding policy when none is named": test.#WorkflowStepExec & _app & {
	parameter: policy: ""
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#MakePlacementDecisions": [{$params: inputs: policyName: "env-bindings"}]
	}
}

"waits until the cloud resource is healthy": test.#WorkflowStepExec & _app & {
	_healthy: false
	expect: {
		phase: "running"
		calls: "vela/op": "#Apply"?: _|_
	}
}

"waits until the connection Secret exists": test.#WorkflowStepExec & _app & {
	resources: []
	expect: {
		phase: "running"
		calls: "vela/op": "#Apply"?: _|_
	}
}

"an env the policy does not declare fails": test.#WorkflowStepExec & _app & {
	parameter: env: "staging"
	expect: {
		phase:   "failed"
		message: =~"undefined field: staging"
		calls: "vela/op": "#Apply"?: _|_
	}
}

"the env is required": test.#WorkflowStepExec & _mocked & {
	parameter: {policy: "env-bindings", placements: [{cluster: "local"}]}
	expect: {
		phase:   "failed"
		message: =~"envName: cannot convert incomplete value"
	}
}

"sharing needs the Application's env-binding policies, so a test mocks them": test.#WorkflowStepExec & {
	definition: "share-cloud-resource"
	parameter: {env: "prod", placements: [{cluster: "eu-1"}]}
	expect: {
		phase:   "failed"
		message: =~"vela/op.#LoadPolicies needs an Application"
	}
}
