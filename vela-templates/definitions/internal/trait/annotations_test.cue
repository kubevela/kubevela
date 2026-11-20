import "vela/test"

_deployment: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: "web", annotations: owner: "shop"}
	spec: template: {
		metadata: labels: app: "web"
		spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"annotates the workload and its pods": test.#TraitRender & {
	definition: "annotations"
	workload:   _deployment
	parameter: "prometheus.io/scrape": "true"
	expect: output: {
		metadata: {
			annotations: {owner: "shop", "prometheus.io/scrape": "true"} @exact()
		}
		spec: template: metadata: {
			labels: app: "web"
			annotations: {
				"prometheus.io/scrape": "true" @exact()
			}
		}
	}
}

"an existing annotation is overridden": test.#TraitRender & {
	definition: "annotations"
	workload:   _deployment
	parameter: owner: "billing"
	expect: output: metadata: annotations: owner: "billing"
}

"a null value removes an annotation": test.#TraitRender & {
	definition: "annotations"
	workload:   _deployment
	parameter: owner: null
	expect: output: metadata: annotations: owner?: _|_
}

"a workload without a pod template gets only its own annotations": test.#TraitRender & {
	definition: "annotations"
	workload: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: level: "debug"}
	parameter: team: "shop"
	expect: output: {
		metadata: annotations: team: "shop"
		data: level: "debug"
		spec?: _|_
	}
}

"a CronJob's job template and its pods are annotated": test.#TraitRender & {
	definition: "annotations"
	workload: {
		apiVersion: "batch/v1"
		kind:       "CronJob"
		metadata: name: "report"
		spec: {
			schedule: "0 * * * *"
			jobTemplate: spec: template: spec: containers: [{name: "report", image: "report:1.0"}]
		}
	}
	parameter: team: "shop"
	expect: output: {
		metadata: annotations: team: "shop"
		spec: {
			template?: _|_
			jobTemplate: {
				metadata: annotations: team: "shop"
				spec: template: metadata: annotations: team: "shop"
			}
		}
	}
}

"a value must be a string": test.#TraitRender & {
	definition: "annotations"
	workload:   _deployment
	parameter: replicas: 3
	expect: error: {
		parameter: [=~"replicas"] @contains()
	}
}
