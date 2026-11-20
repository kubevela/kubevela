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

"cpu alone is both request and limit, and leaves memory alone": test.#TraitRender & _web & {
	parameter: cpu: "500m"
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m"} @exact()
		limits: {cpu: "500m"} @exact()
	}}]
}

"cpu alone keeps the memory the workload already asks for": test.#TraitRender & _web & {
	_workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0", resources: {
			requests: memory: "512Mi"
			limits: memory:   "512Mi"
		}}]
	}
	parameter: cpu: "500m"
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m", memory: "512Mi"} @exact()
		limits: {cpu: "500m", memory: "512Mi"} @exact()
	}}]
}

"cpu alone replaces the cpu the workload asks for, and keeps its memory": test.#TraitRender & _web & {
	_workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0", resources: {
			requests: {cpu: "250m", memory: "512Mi"}
			limits: {cpu: "250m", memory: "512Mi"}
		}}]
	}
	parameter: cpu: "500m"
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m", memory: "512Mi"} @exact()
		limits: {cpu: "500m", memory: "512Mi"} @exact()
	}}]
}

"memory alone replaces the memory the workload asks for, and keeps its cpu": test.#TraitRender & _web & {
	_workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0", resources: {
			requests: {cpu: "250m", memory: "512Mi"}
			limits: {cpu: "250m", memory: "512Mi"}
		}}]
	}
	parameter: memory: "1Gi"
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {cpu: "250m", memory: "1Gi"} @exact()
		limits: {cpu: "250m", memory: "1Gi"} @exact()
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

"memory alone is both request and limit, and leaves cpu alone": test.#TraitRender & _web & {
	parameter: memory: "256Mi"
	expect: output: spec: template: spec: containers: [{resources: {
		requests: {memory: "256Mi"} @exact()
		limits: {memory: "256Mi"} @exact()
	}}]
}
