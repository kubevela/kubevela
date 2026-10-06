import "vela/test"

// Loading components and resolving topology policies need the Application,
// which a step test does not have, so both are mocked.
_app: mocks: {
	"vela/oam": {
		"#LoadComponets": $returns: value: {
			web: {name: "web", type: "webservice", properties: image: "nginx"}
			db: {name: "db", type: "worker", properties: image: "postgres"}
		}
		"#ApplyComponent": $returns: {}
	}
	"vela/multicluster": "#GetPlacementsFromTopologyPolicies": [
		{$params: policies: ["topology-eu"], $returns: placements: [{cluster: "eu-1", namespace: "shop"}]},
		{$params: policies: ["topology-multi"], $returns: placements: [{cluster: "eu-1", namespace: "shop"}, {cluster: "us-1", namespace: "shop-us"}]},
		{$returns: placements: [{cluster: "local", namespace: ""}]},
	]
}

"deploys a component to the cluster its topology policy resolves": test.#WorkflowStepExec & _app & {
	definition: "deploy-components"
	parameter: components: [{name: "web", policies: ["topology-eu"]}]
	expect: {
		phase: "succeeded"
		calls: {
			"vela/multicluster": "#GetPlacementsFromTopologyPolicies": [{$params: policies: ["topology-eu"]}]
			"vela/oam": "#ApplyComponent": [{$params: {
				cluster:   "eu-1"
				namespace: "shop"
				value: {name: "web", type: "webservice", properties: image: "nginx"}
			}}]
		}
	}
}

"deploys a component once per placement": test.#WorkflowStepExec & _app & {
	definition: "deploy-components"
	parameter: components: [{name: "web", policies: ["topology-multi"]}]
	expect: {
		phase: "succeeded"
		calls: "vela/oam": {
			"#ApplyComponent": [
				{$params: {cluster: "eu-1", namespace: "shop", value: name: "web"}},
				{$params: {cluster: "us-1", namespace: "shop-us", value: name: "web"}},
			] @contains()
		}
	}
}

"resolves each component's own policies": test.#WorkflowStepExec & _app & {
	definition: "deploy-components"
	parameter: components: [
		{name: "web", policies: ["topology-eu"]},
		{name: "db", policies: []},
	]
	expect: {
		phase: "succeeded"
		calls: {
			"vela/multicluster": {
				"#GetPlacementsFromTopologyPolicies": [
					{$params: policies: ["topology-eu"]},
					{$params: policies: []},
				] @contains()
			}
			"vela/oam": {
				"#ApplyComponent": [
					{$params: {cluster: "eu-1", namespace: "shop", value: name: "web"}},
					{$params: {cluster: "local", namespace: "", value: name: "db"}},
				] @contains()
			}
		}
	}
}

"leaves components it is not given alone": test.#WorkflowStepExec & _app & {
	definition: "deploy-components"
	parameter: components: [{name: "db", policies: ["topology-eu"]}]
	expect: calls: "vela/oam": "#ApplyComponent": [{$params: value: name: "db"}]
}

"deploys nothing when given no components": test.#WorkflowStepExec & _app & {
	definition: "deploy-components"
	parameter: components: []
	expect: {
		phase: "succeeded"
		calls: {
			"vela/multicluster"?: _|_
			"vela/oam": "#ApplyComponent"?: _|_
		}
	}
}

"fails, deploying nothing, when a component is not in the Application": test.#WorkflowStepExec & _app & {
	definition: "deploy-components"
	parameter: components: [
		{name: "web", policies: ["topology-eu"]},
		{name: "cache", policies: ["topology-eu"]},
		{name: "queue", policies: []},
	]
	expect: {
		phase:   "failed"
		message: "component(s) not found in application: cache, queue"
		calls: {
			"vela/multicluster"?: _|_
			"vela/oam": "#ApplyComponent"?: _|_
		}
	}
}

"loading components needs an Application, so a test mocks it": test.#WorkflowStepExec & {
	definition: "deploy-components"
	parameter: components: [{name: "web", policies: []}]
	expect: {
		phase:   "failed"
		message: =~"needs an Application"
	}
}

// components is a list, so omitting it is an empty list, not a missing value.
"omitting the components deploys nothing": test.#WorkflowStepExec & _app & {
	definition: "deploy-components"
	expect: {
		phase: "succeeded"
		calls: "vela/oam": "#ApplyComponent"?: _|_
	}
}
