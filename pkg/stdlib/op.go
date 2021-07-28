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

var opFile = file{
	name: "op.cue",
	path: "vela/op",
	content: `
#Load: {
  #do: "load"
  component?: string
  workload?: {...}
  auxiliaries?: [...{...}]
}  

#Export: {
  #do: "export"
  type: *"patch" | "var"
  component?: string
  path?: string
  value: _
}

#ConditionalWait: {
  #do: "wait"
  continue: bool
}

#Break: {
  #do: "break"
  message: string
}

#Apply: {
  #do: "apply"
  #provider: "kube"
  patch: _
  ...
}

#Read: {
  #do: "read"
  #provider: "kube"
  result: {...}
  ...
}

#Steps: {
  #do: "steps"
  ...
}

NoExist: _|_

`,
}
