import "vela/test"

_spec: components: [
	{name: "web", type: "webservice", properties: image: "shop:1.0"},
	{name: "worker", type: "worker", properties: image: "shop:1.0", traits: [{type: "scaler", properties: replicas: 2}]},
]

"injects a sidecar into every component": test.#ApplicationPolicyRender & {
	definition: "sidecar-injector"
	context: policyName: "logging"
	spec: _spec
	expect: {
		enabled: true
		output: {
			annotations: "logging.io/injected-by": "logging"
			ctx: logging: sidecar: "fluent-bit:3.0"
		}
		application: {
			metadata: annotations: "logging.io/injected-by": "logging"
			spec: components: [
				{name: "web", traits: [{type: "sidecar", properties: name: "logger"}]},
				// Existing traits are kept, the sidecar after them.
				{name: "worker", traits: [{type: "scaler"}, {type: "sidecar"}]},
			]
		}
	}
}

"an opted-out Application is left alone": test.#ApplicationPolicyRender & {
	definition: "sidecar-injector"
	context: appAnnotations: "logging.io/opt-out": "true"
	spec: _spec
	expect: {
		enabled: false
		application: spec: components: [{name: "web", traits?: _|_}, _]
	}
}

"labels merge with the Application's own": test.#ApplicationPolicyRender & {
	definition: "cost-tag"
	context: appLabels: team: "payments"
	parameter: owner: "shop-team"
	expect: application: metadata: labels: {team: "payments", "platform.io/owner": "shop-team"} @exact()
}

"only the permitted output fields": test.#ApplicationPolicyRender & {
	definition: "leaky"
	expect: error: =~"output.status is not allowed"
}

"integers stay integers": test.#ApplicationPolicyRender & {
	definition: "sidecar-injector"
	spec: components: [{name: "web", type: "webservice", properties: {image: "shop:1.0", replicas: 2}}]
	expect: application: spec: components: [{properties: replicas: 2}]
}
