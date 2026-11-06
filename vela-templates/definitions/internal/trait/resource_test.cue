import "vela/test"

_web: {
	definition: "resource"
	context: name: "web"
	_workload: *_deployment | {...}
	workload: _workload
}

_deployment: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: "web"
	spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
}

"cpu and memory are both request and limit": test.#TraitRender & _web & {
	parameter: {cpu: "500m", memory: "256Mi"}
	expect: {
		output: spec: template: spec: containers: [{
			name:  "web"
			image: "shop:1.0"
			resources: {
				requests: {cpu: "500m", memory: "256Mi"}
				limits: {cpu: "500m", memory: "256Mi"}
			} @exact()
		}]
		outputs: {} @exact()
	}
}

"requests and limits can differ": test.#TraitRender & _web & {
	parameter: {
		requests: {cpu: "250m", memory: "128Mi"}
		limits: {cpu: 1, memory: "1Gi"}
	}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "250m", memory: "128Mi"}
		limits: {cpu: 1, memory: "1Gi"}
	}}]
}

"requests alone set no limits": test.#TraitRender & _web & {
	parameter: requests: {cpu: "250m", memory: "128Mi"}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "250m", memory: "128Mi"}
		limits?: _|_
	}}]
}

"requests and limits take one cpu and 2Gi by default": test.#TraitRender & _web & {
	parameter: {requests: {}, limits: {}}
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: 1, memory: "2048Mi"}
		limits: {cpu: 1, memory: "2048Mi"}
	}}]
}

"cpu alone is both request and limit, with the default memory": test.#TraitRender & _web & {
	parameter: cpu: "500m"
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m", memory: "2048Mi"}
		limits: {cpu: "500m", memory: "2048Mi"}
	}}]
}

"a CronJob's pods are reached through its job template": test.#TraitRender & _web & {
	_workload: {
		apiVersion: "batch/v1"
		kind:       "CronJob"
		metadata: name: "web"
		spec: {
			schedule: "*/5 * * * *"
			jobTemplate: spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
		}
	}
	parameter: {cpu: "500m", memory: "256Mi"}
	expect: output: spec: {
		template?: _|_
		jobTemplate: spec: template: spec: containers: [{name: "web", resources: requests: {cpu: "500m", memory: "256Mi"}}]
	}
}

"memory must carry a unit": test.#TraitRender & _web & {
	parameter: {cpu: "500m", memory: "256"}
	expect: error: {
		parameter: [=~"memory"] @contains()
	}
}

"memory alone is both request and limit, with the default cpu": test.#TraitRender & _web & {
	parameter: memory: "256Mi"
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: 1, memory: "256Mi"}
		limits: {cpu: 1, memory: "256Mi"}
	}}]
}
