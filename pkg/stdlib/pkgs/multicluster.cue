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
		policyName: string
		envName:    string
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

#LoadEnvBindingEnv: #Steps & {
	inputs: {
		env:    string
		policy: string
	}

	loadPolicies: oam.#LoadPolicies @step(1)
	policy_:      string
	if inputs.policy == "" {
		envBindingPolicies: [ for k, v in loadPolicies.value if v.type == "env-binding" {k}]
		policy_: envBindingPolicies[0]
	}
	if inputs.policy != "" {
		policy_: inputs.policy
	}

	loadPolicy: loadPolicies.value["\(policy_)"]
	envMap: {
		for ev in loadPolicy.properties.envs {
			"\(ev.name)": ev
		}
		...
	}
	envConfig_: envMap["\(inputs.env)"]

	outputs: {
		policy:    policy_
		envConfig: envConfig_
	}
}

#PrepareEnvBinding: #Steps & {
	inputs: {
		env:    string
		policy: string
	}
	env_:    inputs.env
	policy_: inputs.policy

	loadEnv: #LoadEnvBindingEnv & {
		inputs: {
			env:    env_
			policy: policy_
		}
	}          @step(1)
	envConfig: loadEnv.outputs.envConfig

	placementDecisions: #MakePlacementDecisions & {
		inputs: {
			policyName: loadEnv.outputs.policy
			envName:    env_
			placement:  envConfig.placement
		}
	} @step(2)

	patchedApp: #PatchApplication & {
		inputs: {
			envName: env_
			if envConfig.selector != _|_ {
				selector: envConfig.selector
			}
			if envConfig.patch != _|_ {
				patch: envConfig.patch
			}
		}
	} @step(3)

	outputs: {
		components: patchedApp.outputs.spec.components
		decisions:  placementDecisions.outputs.decisions
	}
}

#ApplyEnvBindApp: {
	#do: "steps"

	env:       string
	policy:    string
	app:       string
	namespace: string

	env_:    env
	policy_: policy
	prepare: #PrepareEnvBinding & {
		inputs: {
			env:    env_
			policy: policy_
		}
	} @step(1)

	apply: #Steps & {
		for decision in prepare.outputs.decisions {
			for key, comp in prepare.outputs.components {
				"\(decision.cluster)-\(decision.namespace)-\(key)": #ApplyComponent & {
					value: comp
					if decision.cluster != _|_ {
						cluster: decision.cluster
					}
					if decision.namespace != _|_ {
						namespace: decision.namespace
					}
					env: env_
				} @step(2)
			}
		}
	}
}

#ListClusters: {
	#provider: "multicluster"
	#do:       "list-clusters"

	outputs: {
		clusters: [...string]
	}
}
