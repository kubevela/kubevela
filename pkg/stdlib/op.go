/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package stdlib

type file struct {
	name    string
	path    string
	content string
}

var (
	opFile = file{
		name: "op.cue",
		path: "vela/op",
		content: `
import ("encoding/yaml")
#ConditionalWait: {
  #do: "wait"
  continue: bool
}

#Break: {
  #do: "break"
  message: string
}

#Apply: kube.#Apply

#ApplyComponent: #Steps & {
   component: string
   _componentName: component
   load: ws.#Load & {
      component: _componentName
   } @step(1)
   
   workload: workload__.value
   workload__: kube.#Apply & {
      value: load.value.workload
      ...
   } @step(2)
    
   applyTraits__: #Steps & {
      for index,o in load.value.auxiliaries {
          "zz_\(index)": kube.#Apply & {
               value: o
          }
      }
   } @step(3)
}

#ApplyRemaining: #Steps & {
  namespace?: string

  // exceptions specify the resources not to apply.
  exceptions?: [componentName=string]: {
      // skipApplyWorkload indicates whether to skip apply the workload resource
      skipApplyWorkload: *true | bool
      
      // skipAllTraits indicates to skip apply all resources of the traits.
      // If this is true, skipApplyTraits will be ignored
      skipAllTraits: *true| bool

      // skipApplyTraits specifies the names of the traits to skip apply
      skipApplyTraits: [...string]
  }

  components: ws.#Load @step(1)
  #up__: [for name,c in components.value {
        #Steps 
        if exceptions[name] != _|_ {
			   if exceptions[name].skipApplyWorkload == false {
                   "apply-workload": kube.#Apply & {value: c.workload}
			   }
			   if exceptions[name].skipAllTraits == false && c.auxiliaries != _|_ {
				   #up_auxiliaries: [for t in c.auxiliaries {
						kube.#Apply & {value: t}
				   }]
			   }
        }
        if exceptions[name] == _|_ {
			   "apply-workload": kube.#Apply & {value: c.workload}
                if c.auxiliaries != _|_ {
                   #up_auxiliaries:[for index,o in c.auxiliaries {
					   "s\(index)": kube.#Apply & {
						  value: o
					   }
				   }]
                }
				
        }
     }
  ] @step(2)
}

#ApplyEnvBindComponent: #Steps & {
	env:       string
	policy:    string
	component: string
	namespace: string
	_namespace: namespace
	
	envBinding: kube.#Read & {
		value: {
			apiVersion: "core.oam.dev/v1alpha1"
			kind:       "EnvBinding"
			metadata: {
				name:      policy
				namespace: _namespace
			}
		}
	} @step(1)

	// wait until envBinding.value.status equal "finished"
	wait: #ConditionalWait & {
		continue: envBinding.value.status.phase == "finished"
	} @step(2)
	
	configMap: kube.#Read & {
		value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name:      policy
				namespace: _namespace
			}
		}
	} @step(3)

	target: "\(env)-\(component)"
	apply: kube.#Apply & {
		value: {
			yaml.Unmarshal(configMap.value.data[target])
		}
	} @step(4)
}

#Load: ws.#Load

#Read: kube.#Read

#Steps: {
  #do: "steps"
  ...
}

NoExist: _|_

`,
	}
)
