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

var (
	kubeFile = file{
		name: "kube.cue",
		path: "vela/op",
		content: `
kube: {

  #Apply: {
     #do: "apply"
     #provider: "kube"
     value: {...}
     ...
  }

  #Read: {
     #do: "read"
     #provider: "kube"
     value?: {...}
     ...
  }

}

`,
	}

	workspaceFile = file{
		name: "workspace.cue",
		path: "vela/op",
		content: `
ws: {

  #Load: {
    #do: "load"
    component?: string
    value?: {...}
    ...
  }

  #Export: {
    #do: "export"
    component: string
    value: _
 }

  #DoVar: {
    #do: "var"
    method: *"Get" | "Put"
    path: sting
    value?: _
  }

}

`,
	}
)
