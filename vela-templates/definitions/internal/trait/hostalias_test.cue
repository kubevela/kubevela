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
	expect: output: spec: template: spec: {
		hostAliases: [{ip: "10.0.0.1", hostnames: ["db.internal", "postgres.internal"]}] @exact()
	}
}

"a hostname the IP already has is listed once": test.#TraitRender & _web & {
	parameter: hostAliases: [{ip: "10.0.0.1", hostnames: ["db.internal", "postgres.internal", "postgres.internal"]}]
	expect: output: spec: template: spec: {
		hostAliases: [{ip: "10.0.0.1", hostnames: ["db.internal", "postgres.internal"]}] @exact()
	}
}

"a pod with no aliases takes the trait's": test.#TraitRender & _web & {
	_existing: false
	parameter: hostAliases: [{ip: "10.0.0.2", hostnames: ["cache.internal"]}]
	expect: output: spec: template: spec: {
		hostAliases: [{ip: "10.0.0.2", hostnames: ["cache.internal"]}] @exact()
	}
}

"no aliases leave the pod's own alone": test.#TraitRender & _web & {
	expect: output: spec: template: spec: hostAliases: [{ip: "10.0.0.1", hostnames: ["db.internal"]}]
}

"an alias needs an IP": test.#TraitRender & _web & {
	_existing: false
	parameter: hostAliases: [{hostnames: ["cache.internal"]}]
	expect: error: user: ["a host alias needs an ip: cache.internal"]
}

"entries for the same new IP are merged into one": test.#TraitRender & _web & {
	_existing: false
	parameter: hostAliases: [
		{ip: "10.0.0.2", hostnames: ["cache.internal"]},
		{ip: "10.0.0.3", hostnames: ["queue.internal"]},
		{ip: "10.0.0.2", hostnames: ["redis.internal", "cache.internal"]},
	]
	expect: output: spec: template: spec: {
		hostAliases: [
			{ip: "10.0.0.2", hostnames: ["cache.internal", "redis.internal"]},
			{ip: "10.0.0.3", hostnames: ["queue.internal"]},
		] @exact()
	}
}

"a pod alias without hostnames gains the trait's": test.#TraitRender & _web & {
	_existing: false
	workload: spec: template: spec: hostAliases: [{ip: "10.0.0.9"}]
	parameter: hostAliases: [{ip: "10.0.0.9", hostnames: ["metrics.internal"]}]
	expect: output: spec: template: spec: {
		hostAliases: [{ip: "10.0.0.9", hostnames: ["metrics.internal"]}] @exact()
	}
}
