#Placement: {
	clusterSelector?: {
		labels?: [string]: string
		name?: string
	}
	namespaceSelector?: {
		labels?: [string]: string
		name?: string
	}
}

#PlacementDecision: {
	namespace?: string
	cluster?:   string
}

#Component: {
	name: string
	type: string
	properties?: {...}
	traits?: [...{
		type:     string
		disable?: bool
		properties: {...}
	}]
}

#ReadPlacementDecisions: {
	#provider: "multicluster"
	#do:       "read-placement-decisions"

	inputs: {
		policy:  string
		envName: string
	}

	outputs: {
		decisions?: [...#PlacementDecision]
	}
}

#MakePlacementDecisions: {
	#provider: "multicluster"
	#do:       "make-placement-decisions"

	inputs: {
		policyName: string
		envName:    string
		placement:  #Placement
	}

	outputs: {
		decisions: [...#PlacementDecision]
	}
}

#PatchApplication: {
	#provider: "multicluster"
	#do:       "patch-application"

	inputs: {
		envName: string
		patch?: components: [...#Component]
		selector?: components: [...string]
	}

	outputs: {...}
	...
}

#ApplyEnvBindApp: {
	#do: "steps"

	env:       string
	policy:    string
	app:       string
	namespace: string

	loadPolicies: oam.#LoadPolicies @step(1)
	loadPolicy:   loadPolicies.value["\(policy)"]
	envMap: {
		for ev in loadPolicy.properties.envs {
			"\(ev.name)": ev
		}
		...
	}
	envConfig: envMap["\(env)"]

	placementDecisions: multicluster.#MakePlacementDecisions & {
		inputs: {
			policyName: policy
			envName:    env
			placement:  envConfig.placement
		}
	} @step(2)

	patchedApp: multicluster.#PatchApplication & {
		inputs: {
			envName: env
			if envConfig.selector != _|_ {
				selector: envConfig.selector
			}
			if envConfig.patch != _|_ {
				patch: envConfig.patch
			}
		}
	} @step(3)

	components: patchedApp.outputs.spec.components
	apply:      #Steps & {
		for decision in placementDecisions.outputs.decisions {
			for key, comp in components {
				"\(decision.cluster)-\(decision.namespace)-\(key)": #ApplyComponent & {
					value: comp
					if decision.cluster != _|_ {
						cluster: decision.cluster
					}
					if decision.namespace != _|_ {
						namespace: decision.namespace
					}
				} @step(4)
			}
		}
	}
}
