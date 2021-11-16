#LoadTerraformComponents: {
	#provider: "terraform"
	#do:       "load-terraform-components"

	outputs: {
		components: [...multicluster.#Component]
	}
}

#GetConnectionStatus: {
	#provider: "terraform"
	#do:       "get-connection-status"

	inputs: {
		componentName: string
	}

	outputs: {
		healthy?: bool
	}
}

#PrepareTerraformEnvBinding: #Steps & {
	inputs: {
		env:    string
		policy: string
	}
	env_:    inputs.env
	policy_: inputs.policy

	prepare: multicluster.#PrepareEnvBinding & {
		inputs: {
			env:    env_
			policy: policy_
		}
	}                        @step(1)
	loadTerraformComponents: #LoadTerraformComponents @step(2)
	terraformComponentMap: {
		for _, comp in loadTerraformComponents.outputs.components {
			"\(comp.name)": comp
		}
		...
	}
	components_: [ for comp in prepare.outputs.components if terraformComponentMap["\(comp.name)"] != _|_ {comp}]
	outputs: {
		components: components_
		decisions:  prepare.outputs.decisions
	}
}

#loadSecretInfo: {
	component: {...}
	appNamespace: string
	name:         string
	namespace:    string
	env:          string
	if component.properties != _|_ && component.properties.writeConnectionSecretToRef != _|_ {
		if component.properties.writeConnectionSecretToRef.name != _|_ {
			name: component.properties.writeConnectionSecretToRef.name
		}
		if component.properties.writeConnectionSecretToRef.name == _|_ {
			name: component.name
		}
		if component.properties.writeConnectionSecretToRef.namespace != _|_ {
			namespace: component.properties.writeConnectionSecretToRef.namespace
		}
		if component.properties.writeConnectionSecretToRef.namespace == _|_ {
			namespace: appNamespace
		}
	}
	envName: "\(name)-\(env)"
}

#bindTerraformComponentToCluster: #Steps & {
	comp: {...}
	secret: {...}
	env: string
	decisions: [...{...}]

	status: terraform.#GetConnectionStatus & {
		inputs: componentName: "\(comp.name)-\(env)"
	} @step(1)

	read: kube.#Read & {
		value: {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {
				name:      secret.envName
				namespace: secret.namespace
				...
			}
			...
		}
	} @step(2)

	wait: {
		#do:      "wait"
		continue: status.outputs.healthy && read.err == _|_
	} @step(3)

	sync: #Steps & {
		for decision in decisions {
			"\(decision.cluster)-\(decision.namespace)": kube.#Apply & {
				cluster: decision.cluster
				value: {
					apiVersion: "v1"
					kind:       "Secret"
					metadata: {
						name: secret.name
						if decision.namespace != _|_ && decision.namespace != "" {
							namespace: decision.namespace
						}
						if decision.namespace == _|_ || decision.namespace == "" {
							namespace: secret.namespace
						}
						...
					}
					type: "Opaque"
					data: read.value.data
					...
				}
			}
		}
	} @step(4)
}

#DeployCloudResource: {
	#do: "steps"

	env:       string
	name:      string
	policy:    string
	namespace: string

	env_:          env
	policy_:       policy
	prepareDeploy: #PrepareTerraformEnvBinding & {
		inputs: {
			env:    env_
			policy: policy_
		}
	} @step(1)

	deploy: #Steps & {
		for comp in prepareDeploy.outputs.components {
			"\(comp.name)": #Steps & {

				secretMeta: #loadSecretInfo & {
					component:    comp
					env:          env_
					appNamespace: namespace
				}

				apply: #ApplyComponent & {
					value: {
						name: "\(comp.name)-\(env)"
						properties: {
							writeConnectionSecretToRef: {
								name:      secretMeta.envName
								namespace: secretMeta.namespace
							}
							if comp.properties != _|_ {
								for k, v in comp.properties {
									if k != "writeConnectionSecretToRef" {
										"\(k)": v
									}
								}
							}
							...
						}
						for k, v in comp {
							if k != "name" && k != "properties" {
								"\(k)": v
							}
						}
						...
					}
				} @step(1)

				comp_: comp
				bind:  #bindTerraformComponentToCluster & {
					comp:      comp_
					secret:    secretMeta
					env:       env_
					decisions: prepareDeploy.outputs.decisions
				} @step(2)

				secret: bind.read.value

				update: kube.#Apply & {
					value: {
						metadata: {
							for k, v in secret.metadata {
								if k != "labels" {
									"\(k)": v
								}
							}
							labels: {
								"app.oam.dev/name":       name
								"app.oam.dev/namespace":  namespace
								"app.oam.dev/component":  comp.name
								"app.oam.dev/env-name":   env
								"app.oam.dev/sync-alias": secretMeta.name
								if secret.metadata.labels != _|_ {
									for k, v in secret.metadata.labels {
										if k != "app.oam.dev/name" && k != "app.oam.dev/sync-alias" && k != "app.oam.dev/env-name" {
											"\(k)": v
										}
									}
								}
								...
							}
						}
						for k, v in secret {
							if k != "metadata" {
								"\(k)": v
							}
						}
						...
					}
				} @step(6)
			}
		}
		...
	} @step(2)
}

#BindCloudResource: {
	#do: "steps"

	env:        string
	name:       string
	policy:     string
	namespace:  string
	namespace_: namespace
	placements: [...multicluster.#PlacementDecision]

	env_:        env
	policy_:     policy
	prepareBind: #PrepareTerraformEnvBinding & {
		inputs: {
			env:    env_
			policy: policy_
		}
	} @step(1)

	decisions_: [ for placement in placements {
		namespace: *"" | string
		if placement.namespace != _|_ {
			namespace: placement.namespace
		}
		if placement.namespace == _|_ {
			namespace: namespace_
		}
		cluster: *"local" | string
		if placement.cluster != _|_ {
			cluster: placement.cluster
		}
	}]

	deploy: #Steps & {
		for comp in prepareBind.outputs.components {
			"\(comp.name)": #Steps & {
				secretMeta: #loadSecretInfo & {
					component:    comp
					env:          env_
					appNamespace: namespace
				}
				comp_: comp
				bind:  #bindTerraformComponentToCluster & {
					comp:      comp_
					secret:    secretMeta
					env:       env_
					decisions: decisions_
				} @step(1)
			}
		}
	} @step(2)
}
