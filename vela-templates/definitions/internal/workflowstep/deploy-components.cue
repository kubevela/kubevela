import (
	"list"
	"strings"
	"vela/builtin"
	"vela/multicluster"
	"vela/oam"
)

"deploy-components": {
	type: "workflow-step"
	annotations: {
		"category": "Application Delivery"
	}
	labels: {
		"scope": "Application"
	}
	description: "Deploy each component to the cluster(s) resolved from its own topology policies. Applies are executed sequentially, one at a time -- unlike \"deploy\", there is no parallelism setting to configure."
}
template: {
	components: oam.#LoadComponets

	// Iterating (not an indexed lookup) avoids evaluating before "components" resolves.
	_loadedNames: [for name, _ in components.$returns.value {name}]

	_missingComponents: [for entry in parameter.components if !list.Contains(_loadedNames, entry.name) {entry.name}]

	if len(_missingComponents) > 0 {
		validateComponents: builtin.#Fail & {
			$params: message: "component(s) not found in application: \(strings.Join(_missingComponents, ", "))"
		}
	}

	// Gated so nothing is applied unless every component name is valid.
	if len(_missingComponents) == 0 {
		deploy: {
			// "comp", not "value" -- shadows the $params.value field below otherwise.
			for name, comp in components.$returns.value {
				for entry in parameter.components if entry.name == name {
					"\(name)": {
						placements: multicluster.#GetPlacementsFromTopologyPolicies & {
							$params: policies: entry.policies
						}
						apply: {
							for p in placements.$returns.placements {
								"\(p.cluster)-\(p.namespace)": oam.#ApplyComponent & {
									$params: {
										value:     comp
										cluster:   p.cluster
										namespace: p.namespace
									}
								}
							}
						}
					}
				}
			}
		}
	}

	parameter: {
		// +usage=Per-component mapping of which topology policies determine its target cluster(s)
		components: [...{
			// +usage=the name of the component in the application to apply
			name: string
			// +usage=names of topology policies (declared at the Application level) used to resolve this component's target cluster(s)
			policies: [...string]
		}]
	}
}
