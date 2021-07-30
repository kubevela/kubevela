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
#ConditionalWait: {
  #do: "wait"
  continue: bool
}

#Break: {
  #do: "break"
  message: string
}

#Apply: #Steps & {

  object: #KubeApply
  object: patch: _patch
  export: #Export & {
      type: "var"
      path: "applied__"
      value: {"\(object.metadata.namespace)_\(object.apiVersion)_\(object.kind)_\(object.metadata.name)": true}
  }
  _patch?: _
  ...
}

#ApplyComponent: #Steps & {
   componentName: string
   load: #Load & {
      component: componentName
   }
   _applyWorkload: #Apply & {
      object: load.workload
   }
    
   _applyTraits: #Steps & {
      for index,o in load.auxiliaries {
          "s\(index)": #Apply & {
               object: o
          }
      }
   }
}

#ApplyRemaining: #Steps & {
  namespace?: string

  // exceptions specify the resources not to apply.
  exceptions?: {
    [componentName: string]: {
      // skipApplyWorkload indicates whether to skip apply the workload resource
      skipApplyWorkload: *true | bool
      
      // skipAllTraits indicates to skip apply all resources of the traits.
      // If this is true, skipApplyTraits will be ignored
      skipAllTraits: *true| bool

      // skipApplyTraits specifies the names of the traits to skip apply
      skipApplyTraits: [...string]
    }
  }
  
  list: #ListObjects
  applied: #GetVar & {
      path: "applied__"
  } 
  for i,o in list.objects {
     if applied.value["\(o.metadata.namespace)_\(o.apiVersion)_\(o.kind)_\(o.metadata.name)"]==_|_{
        "s\(i)": #KubeApply & {object: o}
     }
  }
}

#Read: #KubeRead

#Steps: {
  #do: "steps"
  ...
}

NoExist: _|_

`,
}
	kubeFile=file{
		name: "kube.cue",
		path: "vela/op",
		content:`
#KubeApply: {
  #do: "apply"
  #provider: "kube"
}


#KubeRead: {
  #do: "read"
  #provider: "kube"
  result?: {...}
  ...
}
`,
	}

	workspaceFile=file{
		name: "workspace.cue",
		path: "vela/op",
		content:`
#Load: {
  #do: "load"
  component?: string
  workload?: {...}
  auxiliaries?: [...{...}]
}  

#ListObjects: {
   #do: "listObjs"
}

#Export: {
  #do: "export"
  type: *"patch" | "var"
  component?: string
  path?: string
  value: _
}

#GetVar: {
  #do: "var"
  method: *"Get" | "Get"
  path: sting
  value?: _
}
`,
	}
)
