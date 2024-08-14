// oam.cue

#ApplyComponent: {
	#provider: "oam"
	#do:       "component-apply"

	$params: {
		// +usage=The cluster to use
		cluster: *"" | string
		// +usage=The env to use
		env: *"" | string
		// +usage=The namespace to apply
		namespace: *"" | string
		// +usage=Whether to wait healthy of the applied component
		waitHealthy: *true | bool
		// +usage=The value of the component resource
		value: {...}
		// +usage=The patcher that will be applied to the resource, you can define the strategy of list merge through comments. Reference doc here: https://kubevela.io/docs/platform-engineers/traits/patch-trait#patch-in-workflow-step
		patch?: {...}
	}

	$returns: {
		output?: {...}
		outputs?: {...}
	}
	...
}

#RenderComponent: {
	#provider: "oam"
	#do:       "component-render"

	$params: {
		cluster:   *"" | string
		env:       *"" | string
		namespace: *"" | string
		value: {...}
		patch?: {...}
	}

	$returns: {
		output?: {...}
		outputs?: {...}
	}
	...
}

#LoadComponets: {
	#provider: "oam"
	#do:       "load"

	$params: {
		// +usage=If specify `app`, use specified application to load its component resources otherwise use current application
		app?: string
	}

	$returns: {
		// +usage=The value of the components will be filled in this field after the action is executed, you can use value[componentName] to refer a specified component
		value?: {...}
	}
	...
}

#LoadPolicies: {
	#provider: "oam"
	#do:       "load-policies"

	$params: {
		// +usage=If specify `app`, use specified application to load its component resources otherwise use current application
		app?: string
	}

	$returns: {
		// +usage=The value of the components will be filled in this field after the action is executed, you can use value[componentName] to refer a specified component
		value?: {...}
	}
	...
}

#LoadComponetsInOrder: {
	#provider: "oam"
	#do:       "load-comps-in-order"

	$params: {
		// +usage=If specify `app`, use specified application to load its component resources otherwise use current application
		app?: string
	}

	$returns: {
		// +usage=The value of the components will be filled in this field after the action is executed, you can use value[componentName] to refer a specified component
		value?: {...}
	}
	...
}

#Load: #LoadComponets

#Apply: #ApplyComponent

#LoadInOrder: #LoadComponetsInOrder

#ApplyApplication: #Steps & {
	load:       #LoadComponetsInOrder
	components: #Steps & {
		for name, c in load.$returns.value {
			"\(name)": #ApplyComponent & {
				$params: value: c
			}
		}
	}
}

// This operator will dispatch all the components in parallel when applying an application.
// Currently it works for Addon Observability to speed up the installation. It can also works for other applications, which
// needs to skip health check for components.
#ApplyApplicationInParallel: #Steps & {
	load:       #LoadComponetsInOrder
	components: #Steps & {
		for name, c in load.$returns.value {
			"\(name)": #ApplyComponent & {
				$params: {
					value:       c
					waitHealthy: false
				}
			}
		}
	}
}

#ApplyComponentRemaining: #Steps & {
	// exceptions specify the resources not to apply.
	exceptions: [...string]
	exceptions_: {for c in exceptions {"\(c)": true}}
	component: string

	load:   #LoadComponets
	render: #Steps & {
		rendered: #RenderComponent & {
			$params: value: load.$returns.value[component]
		}
		comp: #Apply & {
			$params: value: rendered.output
		}
		for name, c in rendered.$returns.outputs {
			if exceptions_[name] == _|_ {
				"\(name)": #Apply & {
					$params: value: c
				}
			}
		}
	}
}

#ApplyRemaining: #Steps & {
	// exceptions specify the resources not to apply.
	exceptions: [...string]
	exceptions_: {for c in exceptions {"\(c)": true}}

	load:       #LoadComponets
	components: #Steps & {
		for name, c in load.$returns.value {
			if exceptions_[name] == _|_ {
				"\(name)": #ApplyComponent & {
					$params: value: c
				}
			}
		}
	}
}
