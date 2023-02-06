// deprecated
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

// deprecated
#PlacementDecision: {
	namespace?: string
	cluster?:   string
}

// deprecated
#Component: {
	name?: string
	type?: string
	properties?: {...}
	traits?: [...{
		type:     string
		disable?: bool
		properties: {...}
	}]
	externalRevision?: string
	dependsOn?: [...string]
}

// deprecated
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

// deprecated
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

// deprecated
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

// deprecated
#LoadEnvBindingEnv: #Steps & {
	inputs: {
		env:    string
		policy: string
	}

	loadPolicies: oam.#LoadPolicies @step(1)
	policy_:      string
	envBindingPolicies: []
	if inputs.policy == "" && loadPolicies.value != _|_ {
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

// deprecated
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

// deprecated
#ApplyComponentsToEnv: #Steps & {
	inputs: {
		decisions: [...#PlacementDecision]
		components: [...#Component]
		env:         string
		waitHealthy: bool
	} @step(1)

	outputs: #Steps & {
		for decision in inputs.decisions {
			for key, comp in inputs.components {
				"\(decision.cluster)-\(decision.namespace)-\(key)": #ApplyComponent & {
					value: comp
					if decision.cluster != _|_ {
						cluster: decision.cluster
					}
					if decision.namespace != _|_ {
						namespace: decision.namespace
					}
					waitHealthy: inputs.waitHealthy
					env:         inputs.env
				} @step(1)
			}
		}
	} @step(2)
}

// deprecated
#ApplyEnvBindApp: {
	#do: "steps"

	env:       string
	policy:    string
	app:       string
	namespace: string
	parallel:  bool

	env_:    env
	policy_: policy
	prepare: #PrepareEnvBinding & {
		inputs: {
			env:    env_
			policy: policy_
		}
	} @step(1)

	apply: #ApplyComponentsToEnv & {
		inputs: {
			decisions:   prepare.outputs.decisions
			components:  prepare.outputs.components
			env:         env_
			waitHealthy: !parallel
		}
	} @step(2)

	if parallel {
		wait: #ApplyComponentsToEnv & {
			inputs: {
				decisions:   prepare.outputs.decisions
				components:  prepare.outputs.components
				env:         env_
				waitHealthy: true
			}
		} @step(3)
	}
}

#ListClusters: {
	#provider: "multicluster"
	#do:       "list-clusters"

	outputs: {
		clusters: [...string]
	}
}

#GetPlacementsFromTopologyPolicies: {
	#provider: "multicluster"
	#do:       "get-placements-from-topology-policies"
	policies: [...string]
	placements: [...{
		cluster:   string
		namespace: string
	}]
}

#Deploy: {
	#provider: "multicluster"
	#do:       "deploy"
	policies: [...string]
	parallelism:              int
	ignoreTerraformComponent: bool
	inlinePolicies:           *[] | [...{...}]
}
