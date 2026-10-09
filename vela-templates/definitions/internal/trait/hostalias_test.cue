import "vela/test"

_web: {
	definition: "hostalias"
	_existing:  *true | bool
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: {
			containers: [{name: "web", image: "shop:1.0"}]
			if _existing {
				hostAliases: [{ip: "10.0.0.1", hostnames: ["db.internal"]}]
			}
		}
	}
}

"adds host aliases beside those the pod has": test.#TraitRender & _web & {
	parameter: hostAliases: [{ip: "10.0.0.2", hostnames: ["cache.internal", "redis.internal"]}]
	expect: {
		output: spec: template: spec: {
			containers: [{name: "web", image: "shop:1.0"}]
			hostAliases: [
				{ip: "10.0.0.1", hostnames: ["db.internal"]},
				{ip: "10.0.0.2", hostnames: ["cache.internal", "redis.internal"]},
			] @contains()
		}
		outputs: {} @exact()
	}
}

"an alias for an IP the pod already has is merged into it by IP": test.#TraitRender & _web & {
	parameter: hostAliases: [{ip: "10.0.0.1", hostnames: ["postgres.internal"]}]
	expect: output: spec: template: spec: hostAliases: [{ip: "10.0.0.1", hostnames: ["db.internal", "postgres.internal"] @contains()}]
} @pending(entries merged by ip unify their hostnames lists element by element, so a different hostname for a known IP conflicts)

"no aliases leave the pod's own alone": test.#TraitRender & _web & {
	expect: output: spec: template: spec: hostAliases: [{ip: "10.0.0.1", hostnames: ["db.internal"]}]
}

"an alias needs an IP": test.#TraitRender & _web & {
	_existing: false
	parameter: hostAliases: [{hostnames: ["cache.internal"]}]
	expect: error: =~"ip"
}
