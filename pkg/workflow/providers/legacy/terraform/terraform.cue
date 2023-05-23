// terraform.cue

#LoadTerraformComponents: {
	#provider: "op"
	#do:       "load-terraform-components"

	outputs: {
		components: [...#Component]
	}
}

#GetConnectionStatus: {
	#provider: "op"
	#do:       "get-connection-status"

	inputs: {
		componentName: string
	}

	outputs: {
		healthy?: bool
	}
}

#PrepareTerraformEnvBinding: {
	inputs: {
		env:    string
		policy: string
	}
	env_:    inputs.env
	policy_: inputs.policy

	prepare: #PrepareEnvBinding & {
		inputs: {
			env:    env_
			policy: policy_
		}
	}
	loadTerraformComponents: #LoadTerraformComponents
	terraformComponentMap: {
		for _, comp in loadTerraformComponents.outputs.components {
			(comp.name): comp
		}
		...
	}
	components_: [ for comp in prepare.outputs.components if terraformComponentMap[(comp.name)] != _|_ {comp}]
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
	if component.properties != _|_ if component.properties.writeConnectionSecretToRef != _|_ {
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

#bindTerraformComponentToCluster: {
	comp: {...}
	secret: {...}
	env: string
	decisions: [...{...}]

	status: #GetConnectionStatus & {
		inputs: componentName: "\(comp.name)-\(env)"
	}

	read: #Read & {
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
	}

	wait: #ConditionalWait & {
		continue: status.outputs.healthy && read.err == _|_
	}

	sync: {
		for decision in decisions {
			"\(decision.cluster)-\(decision.namespace)": #Apply & {
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
	}
}

#DeployCloudResource: {
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
	}

	deploy: {
		for comp in prepareDeploy.outputs.components {
			(comp.name): {

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
										(k): v
									}
								}
							}
							...
						}
						for k, v in comp {
							if k != "name" && k != "properties" {
								(k): v
							}
						}
						...
					}
				}

				comp_: comp
				bind:  #bindTerraformComponentToCluster & {
					comp:      comp_
					secret:    secretMeta
					env:       env_
					decisions: prepareDeploy.outputs.decisions
				}

				secret: bind.read.value

				update: #Apply & {
					value: {
						metadata: {
							for k, v in secret.metadata {
								if k != "labels" {
									(k): v
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
											(k): v
										}
									}
								}
								...
							}
						}
						for k, v in secret {
							if k != "metadata" {
								(k): v
							}
						}
						...
					}
				}
			}
		}
		...
	}
}

#ShareCloudResource: {
	env:        string
	name:       string
	policy:     string
	namespace:  string
	namespace_: namespace
	placements: [...#PlacementDecision]

	env_:        env
	policy_:     policy
	prepareBind: #PrepareTerraformEnvBinding & {
		inputs: {
			env:    env_
			policy: policy_
		}
	}

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

	deploy: {
		for comp in prepareBind.outputs.components {
			(comp.name): {
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
				}
			}
		}
	}
}
