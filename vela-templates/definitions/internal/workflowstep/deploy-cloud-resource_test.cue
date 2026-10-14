import (
	"vela/kube"
	"vela/test"
)

// The step is vela/op's legacy #DeployCloudResource. What an env-binding
// Application supplies (its policies, the env's placements and patched
// components, which of them are terraform, and their health) is mocked; the
// connection Secret it reads and the copies it writes are real.
namespaces: {
	app: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "cloud-deploy"}}
	apps: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "cloud-deploy-apps"}}
} @before()

// The copies are the step's, so each case tidies them.
tidy: {
	db: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-deploy"}}}
	cache: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "Secret", metadata: {name: "cache", namespace: "cloud-deploy"}}}
	apps: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-deploy-apps"}}}
} @afterEach()

_db: {name: "db", type: "alibaba-rds", properties: {instance: "small", writeConnectionSecretToRef: name: "db-conn"}}
_cache: {name: "cache", type: "alibaba-redis", properties: writeConnectionSecretToRef: {}}
_web: {name: "web", type: "webservice", properties: image: "nginx"}

_connection: {apiVersion: "v1", kind: "Secret", metadata: namespace: *"cloud-deploy" | string, type: "Opaque", data: password: "czNjcmV0"}

_app: _mocked & {parameter: {env: *"prod" | string, policy: *"env-bindings" | string}}

_mocked: {
	_components: *[_db] | [...]
	_terraform: *[_db] | [...]
	_decisions: *[{cluster: "local", namespace: ""}] | [...]
	_healthy:   *true | bool
	definition: "deploy-cloud-resource"
	context: {appName: "shop", namespace: "cloud-deploy"}
	resources: *[_connection & {metadata: name: "db-conn-prod"}] | [...]
	mocks: "vela/op": {
		"#LoadPolicies": $returns: value: "env-bindings": {type: "env-binding", properties: envs: [
			{name: "prod", placement: clusterSelector: name: "local"},
			{name: "test", placement: clusterSelector: name: "local"},
		]}
		// Only an env with a placement is answered; any other reaches the
		// provider and fails there.
		"#MakePlacementDecisions": {
			$params: inputs: placement: clusterSelector: name: "local"
			$returns: outputs: decisions: _decisions
		}
		"#PatchApplication": $returns: outputs: spec: components: _components
		"#LoadTerraformComponents": $returns: outputs: components: _terraform
		"#ApplyComponent": $returns: {}
		"#GetConnectionStatus": $returns: outputs: healthy: _healthy
	}
}

"succeeds once it has labelled the connection Secret": test.#WorkflowStepExec & _app & {
	expect: phase: "succeeded"
}

"deploys the terraform component for the env and labels its connection Secret": test.#WorkflowStepExec & _app & {
	expect: {
		calls: "vela/op": {
			"#MakePlacementDecisions": [{$params: inputs: {policyName: "env-bindings", envName: "prod", placement: clusterSelector: name: "local"}}]
			"#ApplyComponent": [{$params: value: {
				name: "db-prod"
				type: "alibaba-rds"
				properties: {
					instance: "small"
					writeConnectionSecretToRef: {name: "db-conn-prod", namespace: "cloud-deploy"}
				}
			}}]
			"#GetConnectionStatus": [{$params: inputs: componentName: "db-prod"}]
		}
		resources: [{
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {
				name: "db-conn-prod"
				labels: {
					"app.oam.dev/name":       "shop"
					"app.oam.dev/namespace":  "cloud-deploy"
					"app.oam.dev/component":  "db"
					"app.oam.dev/env-name":   "prod"
					"app.oam.dev/sync-alias": "db-conn"
				}
			}
			data: password: "czNjcmV0"
		}, {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: name: "db-conn"
			type: "Opaque"
			data: password: "czNjcmV0"
		}]
	}
}

"uses the first env-binding policy when none is named": test.#WorkflowStepExec & _app & {
	parameter: policy: ""
	expect: {
		phase: "succeeded"
		calls: "vela/op": "#MakePlacementDecisions": [{$params: inputs: policyName: "env-bindings"}]
	}
}

"deploys only the components that are terraform": test.#WorkflowStepExec & _app & {
	_components: [_web, _db]
	expect: {
		calls: "vela/op": "#ApplyComponent": [{$params: value: name: "db-prod"}]
	}
}

"does nothing when the env has no terraform components": test.#WorkflowStepExec & _app & {
	_components: [_web]
	_terraform: []
	resources: []
	expect: {
		phase: "succeeded"
		calls: "vela/op": {
			"#ApplyComponent"?:      _|_
			"#GetConnectionStatus"?: _|_
			"#Apply"?:               _|_
		}
	}
}

"deploys every terraform component with a Secret named after it by default": test.#WorkflowStepExec & _app & {
	_components: [_db, _cache]
	_terraform: [_db, _cache]
	resources: [_connection & {metadata: name: "db-conn-prod"}, _connection & {metadata: name: "cache-prod"}]
	expect: {
		calls: "vela/op": {
			"#ApplyComponent": [
				{$params: value: {name: "db-prod", properties: writeConnectionSecretToRef: name: "db-conn-prod"}},
				{$params: value: {name: "cache-prod", properties: writeConnectionSecretToRef: name: "cache-prod"}},
			] @contains()
		}
		resources: [
			{apiVersion: "v1", kind: "Secret", metadata: name: "db-conn", data: password: "czNjcmV0"},
			{apiVersion: "v1", kind: "Secret", metadata: name: "cache", data: password: "czNjcmV0"},
		]
	}
}

"writes the connection Secret to the namespace the component names": test.#WorkflowStepExec & _app & {
	_components: [_db & {properties: writeConnectionSecretToRef: namespace: "cloud-deploy-apps"}]
	_terraform: _components
	resources: [_connection & {metadata: {name: "db-conn-prod", namespace: "cloud-deploy-apps"}}]
	expect: {
		calls: "vela/op": "#ApplyComponent": [{$params: value: properties: writeConnectionSecretToRef: {name: "db-conn-prod", namespace: "cloud-deploy-apps"}}]
		resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-deploy-apps"}, data: password: "czNjcmV0"}]
	}
}

"copies the connection Secret to every cluster and namespace the env is placed on": test.#WorkflowStepExec & _app & {
	_decisions: [{cluster: "local", namespace: "cloud-deploy-apps"}, {cluster: "eu-1", namespace: ""}]
	mocks: "vela/op": "#Apply": {$params: cluster: "eu-1", $returns: {}}
	expect: {
		calls: "vela/op": {
			"#Apply": [
				{$params: {cluster: "local", value: metadata: {name: "db-conn", namespace: "cloud-deploy-apps"}}},
				{$params: {cluster: "eu-1", value: metadata: {name: "db-conn", namespace: "cloud-deploy"}}},
				{$params: value: metadata: name: "db-conn-prod"},
			] @contains()
		}
		resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn", namespace: "cloud-deploy-apps"}, data: password: "czNjcmV0"}]
	}
}

"keeps the connection Secret's own labels but not a stale Application's": test.#WorkflowStepExec & _app & {
	resources: [_connection & {metadata: {name: "db-conn-prod", labels: {"app.oam.dev/name": "old-shop", team: "data"}}}]
	expect: {
		resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn-prod", labels: {"app.oam.dev/name": "shop", team: "data"}}}]
	}
}

"waits until the cloud resource is healthy": test.#WorkflowStepExec & _app & {
	_healthy: false
	expect: {
		phase: "running"
		calls: "vela/op": "#ApplyComponent": [{$params: value: name: "db-prod"}]
		resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "db-conn-prod", labels?: _|_}}]
	}
}

"waits until the connection Secret exists": test.#WorkflowStepExec & _app & {
	resources: []
	expect: {
		phase: "running"
		calls: "vela/op": "#ApplyComponent": [{$params: value: name: "db-prod"}]
	}
}

"an env the policy does not declare fails": test.#WorkflowStepExec & _app & {
	parameter: env: "staging"
	expect: {
		phase:   "failed"
		message: =~"undefined field: staging"
		calls: "vela/op": "#ApplyComponent"?: _|_
	}
}

"the env is required": test.#WorkflowStepExec & _mocked & {
	parameter: policy: "env-bindings"
	expect: {
		phase:   "failed"
		message: =~"envName: cannot convert incomplete value"
	}
}

"loading the Application's policies needs an Application, so a test mocks it": test.#WorkflowStepExec & {
	definition: "deploy-cloud-resource"
	parameter: env: "prod"
	expect: {
		phase:   "failed"
		message: =~"vela/op.#LoadPolicies needs an Application"
	}
}
