// multicluster.cue

#ListClusters: {
	#provider: "op"
	#do:       "list-clusters"

	outputs: {
		clusters: [...string]
	}
}

#GetPlacementsFromTopologyPolicies: {
	#provider: "op"
	#do:       "get-placements-from-topology-policies"
	policies: [...string]
	placements: [...{
		cluster:   string
		namespace: string
	}]
}

#Deploy: {
	#provider: "op"
	#do:       "deploy"
	policies: [...string]
	parallelism:              int
	ignoreTerraformComponent: bool
	inlinePolicies: *[] | [...{...}]
}

// deprecated
#MakePlacementDecisions: {
	#provider: "op"
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
	#provider: "op"
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
#LoadEnvBindingEnv: {
	inputs: {
		env:    string
		policy: string
	}

	loadPolicies: #LoadPolicies
	policy_:      string
	envBindingPolicies: []
	if inputs.policy == "" && loadPolicies.value != _|_ {
		envBindingPolicies: [for k, v in loadPolicies.value if v.type == "env-binding" {k}]
		if len(envBindingPolicies) > 0 {
			policy_: envBindingPolicies[0]
		}
	}
	if inputs.policy != "" {
		policy_: inputs.policy
	}

	loadPolicy: loadPolicies.value[(policy_)]
	envMap: {
		for ev in loadPolicy.properties.envs {
			(ev.name): ev
		}
		...
	}
	envConfig_: envMap[(inputs.env)]

	outputs: {
		policy:    policy_
		envConfig: envConfig_
	}
}

// deprecated
#PrepareEnvBinding: {
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
	}
	envConfig: loadEnv.outputs.envConfig

	placementDecisions: #MakePlacementDecisions & {
		inputs: {
			policyName: loadEnv.outputs.policy
			envName:    env_
			placement:  envConfig.placement
		}
	}

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
	}

	outputs: {
		components: patchedApp.outputs.spec.components
		decisions:  placementDecisions.outputs.decisions
	}
}

// deprecated
#ApplyComponentsToEnv: {
	inputs: {
		decisions: [...#PlacementDecision]
		components: [...#Component]
		env:         string
		waitHealthy: bool
	}

	outputs: {
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
				}
			}
		}
	}
}

// deprecated
#ApplyEnvBindApp: {
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
	}

	apply: #ApplyComponentsToEnv & {
		inputs: {
			decisions:   prepare.outputs.decisions
			components:  prepare.outputs.components
			env:         env_
			waitHealthy: !parallel
		}
	}

	if parallel {
		wait: #ApplyComponentsToEnv & {
			inputs: {
				decisions:   prepare.outputs.decisions
				components:  prepare.outputs.components
				env:         env_
				waitHealthy: true
			}
		}
	}
}
