#GetPlacementsFromTopologyPolicies: multicluster.#GetPlacementsFromTopologyPolicies

#Deploy: multicluster.#Deploy

#ApplyApplication: #Steps & {
	load:       oam.#LoadComponetsInOrder @step(1)
	components: #Steps & {
		for name, c in load.value {
			"\(name)": oam.#ApplyComponent & {
				value: c
			}
		}
	} @step(2)
}

// This operator will dispatch all the components in parallel when applying an application.
// Currently it works for Addon Observability to speed up the installation. It can also works for other applications, which
// needs to skip health check for components.
#ApplyApplicationInParallel: #Steps & {
	load:       oam.#LoadComponetsInOrder @step(1)
	components: #Steps & {
		for name, c in load.value {
			"\(name)": oam.#ApplyComponent & {
				value:       c
				waitHealthy: false
			}
		}
	} @step(2)
}

#ApplyComponent: oam.#ApplyComponent

#RenderComponent: oam.#RenderComponent

#ApplyComponentRemaining: #Steps & {
	// exceptions specify the resources not to apply.
	exceptions: [...string]
	exceptions_: {for c in exceptions {"\(c)": true}}
	component: string

	load:   oam.#LoadComponets @step(1)
	render: #Steps & {
		rendered: oam.#RenderComponent & {
			value: load.value[component]
		}
		comp: kube.#Apply & {
			value: rendered.output
		}
		for name, c in rendered.outputs {
			if exceptions_[name] == _|_ {
				"\(name)": kube.#Apply & {
					value: c
				}
			}
		}
	} @step(2)
}

#ApplyRemaining: #Steps & {
	// exceptions specify the resources not to apply.
	exceptions: [...string]
	exceptions_: {for c in exceptions {"\(c)": true}}

	load:       oam.#LoadComponets @step(1)
	components: #Steps & {
		for name, c in load.value {
			if exceptions_[name] == _|_ {
				"\(name)": oam.#ApplyComponent & {
					value: c
				}
			}

		}
	} @step(2)
}

#ApplyEnvBindApp: multicluster.#ApplyEnvBindApp

#DeployCloudResource: terraform.#DeployCloudResource

#ShareCloudResource: terraform.#ShareCloudResource

#LoadPolicies: oam.#LoadPolicies

#ListClusters: multicluster.#ListClusters

#MakePlacementDecisions: multicluster.#MakePlacementDecisions

#PatchApplication: multicluster.#PatchApplication

#Load: oam.#LoadComponets

#LoadInOrder: oam.#LoadComponetsInOrder
