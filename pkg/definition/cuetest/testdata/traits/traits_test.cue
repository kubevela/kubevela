import "vela/test"

_app: {
	definition: "app"
	parameter: image: "nginx"
}

"traits patch the workload and add outputs": test.#ComponentRender & _app & {
	traits: [
		{definition: "../defs/scaler", parameter: replicas: 3},
		{definition: "team", parameter: team: "payments"},
	]
	expect: {
		output: {
			spec: replicas: 3
			metadata: labels: team: "payments"
		}
		outputs: service: kind: "Service"
		outputs: hpa?:          _|_
		traits: {
			scaler: outputs: hpa: kind: "HorizontalPodAutoscaler"
			team: outputs: cfg: data: team: "payments"
		}
	}
}

"a trait overrides a parameter default": test.#ComponentRender & {
	definition: "../defs/web"
	parameter: image: "nginx"
	traits: [{definition: "../defs/scaler", parameter: replicas: 3}]
	expect: output: spec: replicas: 3
}

"a trait conflicts with a value the user set": test.#ComponentRender & {
	definition: "../defs/web"
	parameter: {image: "nginx", replicas: 1}
	traits: [{definition: "../defs/scaler", parameter: replicas: 3}]
	expect: error: =~"conflicting values 3 and 1"
}

"the same trait twice, with clashing outputs": test.#ComponentRender & _app & {
	traits: [
		{definition: "team", parameter: team: "payments"},
		{definition: "team", parameter: team: "payments"},
	]
	expect: error: =~"two team traits both render outputs.cfg"
}

_withTeam: _app & {
	traits: [{definition: "team", parameter: team: "payments"}]
}

"component and trait both healthy": test.#ComponentStatus & _withTeam & {
	observed: {
		output: status: readyReplicas: 1
		traits: team: outputs: cfg: metadata: annotations: synced: "true"
	}
	expect: {
		healthy: true
		traits: team: {healthy: true, message: "team payments"}
	}
}

"trait unhealthy, component fine": test.#ComponentStatus & _withTeam & {
	observed: {
		output: status: readyReplicas: 1
		traits: team: outputs: cfg: metadata: annotations: synced: "false"
	}
	expect: {
		healthy: true
		traits: team: healthy: false
	}
}

"observed a trait that is not attached": test.#ComponentStatus & _withTeam & {
	observed: traits: scaler: outputs: hpa: {}
	expect: error: =~"observed.traits.scaler: no scaler trait renders outputs"
}

"two sources render the same object": test.#ComponentStatus & _app & {
	traits: [{definition: "svc"}]
	expect: error: =~"outputs.service and traits.svc.outputs.service both render Service default/test-component"
}

"context: a later trait's output replaces the component's": test.#ComponentRender & _app & {
	traits: [{definition: "team", parameter: team: "payments"}, {definition: "svc"}]
	expect: {
		outputs: service: metadata: labels: "trait.oam.dev/type": "AuxiliaryWorkload"
		traits: svc: outputs: service: metadata: labels: "trait.oam.dev/type": "svc"
		outputs: service: metadata: annotations: from?: _|_  // the component's own, as applied
		context: outputs: {
			service: metadata: annotations: from: "svc"   // svc's service replaced the component's
			service: metadata: labels?: _|_               // templates see outputs before the controller labels them
			cfg: kind: "ConfigMap"
		}
	}
}

"context is what the templates saw": test.#ComponentRender & _app & {
	context: {name: "api", appName: "shop"}
	traits: [{definition: "team", parameter: team: "payments"}]
	expect: context: {
		name:    "api"
		appName: "shop"
		output: {
			metadata: labels: team: "payments"          // after the trait's patch...
			metadata: labels: "app.oam.dev/name"?: _|_  // ...but before the controller's labels
		}
		outputs: {service: _, cfg: _} @exact()
	}
}

"an unreadable context only matters when asked about": test.#ComponentRender & {
	definition: "open"
	expect: output: kind: "ConfigMap"
}

"asking about an unreadable context fails": test.#ComponentRender & {
	definition: "open"
	expect: context: name: "test-component"
}
