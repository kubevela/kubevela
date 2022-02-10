import (
	"vela/op"
)

"deploy": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Deploy components with policies."
}
template: {
	deploy: op.#Steps & {
		load: op.#Load @step(1)
		_components: [ for k, v in load.value {v}]
		loadPoliciesInOrder: op.#LoadPoliciesInOrder & {
			if parameter.policies != _|_ {
						input: parameter.policies
					}
		}                     @step(2)
		_policies:            loadPoliciesInOrder.output
		handleDeployPolicies: op.#HandleDeployPolicies & {
			inputs: {
				components: _components
				policies:   _policies
			}
		}                   @step(3)
		_decisions:         handleDeployPolicies.outputs.decisions
		_patchedComponents: handleDeployPolicies.outputs.components
		deploy:             op.#Steps & {
			for decision in _decisions {
				for key, comp in _patchedComponents {
					"\(decision.cluster)-\(decision.namespace)-\(key)": op.#ApplyComponent & {
						value: comp
						if decision.cluster != _|_ {
							cluster: decision.cluster
						}
						if decision.namespace != _|_ {
							namespace: decision.namespace
						}
					} @step(1)
				}
			}
		} @step(4)
	}
	parameter: {
		policies?: [...string]
	}
}
