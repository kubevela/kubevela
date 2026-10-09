import "vela/test"

_deployment: {
	definition: "k8s-update-strategy"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: {
			replicas: 3
			strategy: {type: "RollingUpdate", rollingUpdate: {maxSurge: 1, maxUnavailable: 0}}
			template: spec: containers: [{name: "web", image: "shop:1.0"}]
		}
	}
}

_statefulset: {
	definition: "k8s-update-strategy"
	workload: {
		apiVersion: "apps/v1"
		kind:       "StatefulSet"
		metadata: name: "db"
		spec: template: spec: containers: [{name: "db", image: "postgres:16"}]
	}
}

_daemonset: {
	definition: "k8s-update-strategy"
	workload: {
		apiVersion: "apps/v1"
		kind:       "DaemonSet"
		metadata: name: "agent"
		spec: template: spec: containers: [{name: "agent", image: "agent:1.0"}]
	}
}

"a Deployment rolls out with the given surge and unavailability": test.#TraitRender & _deployment & {
	parameter: strategy: rollingStrategy: {maxSurge: "50%", maxUnavailable: "0"}
	expect: output: spec: {
		replicas: 3
		strategy: {type: "RollingUpdate", rollingUpdate: {maxSurge: "50%", maxUnavailable: "0"} @exact()} @exact()
		updateStrategy?: _|_
	}
}

"rolling update bounds default to a quarter": test.#TraitRender & _deployment & {
	parameter: strategy: rollingStrategy: {}
	expect: output: spec: strategy: rollingUpdate: {maxSurge: "25%", maxUnavailable: "25%"}
}

"a rolling update needs no parameters": test.#TraitRender & _deployment & {
	expect: output: spec: strategy: {type: "RollingUpdate", rollingUpdate: {maxSurge: "25%", maxUnavailable: "25%"}}
}

"Recreate drops the Deployment's rolling update settings": test.#TraitRender & _deployment & {
	parameter: strategy: type: "Recreate"
	expect: output: spec: {
		strategy: {type: "Recreate"} @exact()
	}
}

"OnDelete leaves a Deployment's strategy alone": test.#TraitRender & _deployment & {
	parameter: strategy: type: "OnDelete"
	expect: output: spec: {
		strategy: {type: "RollingUpdate", rollingUpdate: {maxSurge: 1, maxUnavailable: 0}} @exact()
	}
}

"a StatefulSet rolls out from a partition": test.#TraitRender & _statefulset & {
	parameter: {targetKind: "StatefulSet", strategy: rollingStrategy: partition: 2}
	expect: output: spec: {
		updateStrategy: {type: "RollingUpdate", rollingUpdate: {partition: 2} @exact()} @exact()
		strategy?: _|_
	}
}

"a StatefulSet can wait for pods to be deleted": test.#TraitRender & _statefulset & {
	parameter: {targetKind: "StatefulSet", strategy: type: "OnDelete"}
	expect: output: spec: {
		updateStrategy: {type: "OnDelete"} @exact()
	}
}

"Recreate leaves a StatefulSet alone": test.#TraitRender & _statefulset & {
	parameter: {targetKind: "StatefulSet", strategy: type: "Recreate"}
	expect: output: spec: {updateStrategy?: _|_, strategy?: _|_}
}

"a DaemonSet rolls out with the given surge and unavailability": test.#TraitRender & _daemonset & {
	parameter: {targetKind: "DaemonSet", strategy: rollingStrategy: {maxSurge: "1", maxUnavailable: "0"}}
	expect: output: spec: {
		updateStrategy: {type: "RollingUpdate", rollingUpdate: {maxSurge: "1", maxUnavailable: "0"} @exact()} @exact()
		strategy?: _|_
	}
}

"a DaemonSet can wait for pods to be deleted": test.#TraitRender & _daemonset & {
	parameter: {targetKind: "DaemonSet", strategy: type: "OnDelete"}
	expect: output: spec: {
		updateStrategy: {type: "OnDelete"} @exact()
	}
}

"only Deployments, StatefulSets and DaemonSets are targeted": test.#TraitRender & _deployment & {
	parameter: targetKind: "Job"
	expect: error: {
		parameter: [=~"targetKind"] @contains()
	}
}

"a StatefulSet rolls out from partition zero with no rolling settings": test.#TraitRender & _statefulset & {
	parameter: targetKind: "StatefulSet"
	expect: output: spec: updateStrategy: {type: "RollingUpdate", rollingUpdate: partition: 0}
}
