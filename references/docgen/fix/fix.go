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

package fix

import (
	"encoding/json"

	"github.com/oam-dev/kubevela/apis/types"
)

var (
	// CapContainerImage is the cap for container image
	CapContainerImage *types.Capability
)

// FIXME: remove this temporary fix when https://github.com/cue-lang/cue/issues/2047 is fixed
func init() {
	legacyJSON := `{"name":"container-image","type":"trait","template":"#PatchParams: {\n\t// +usage=Specify the name of the target container, if not set, use the component name\n\tcontainerName: *\"\" | string\n\t// +usage=Specify the image of the container\n\timage: string\n\t// +usage=Specify the image pull policy of the container\n\timagePullPolicy: *\"\" | \"IfNotPresent\" | \"Always\" | \"Never\"\n}\nPatchContainer: {\n\t_params:         #PatchParams\n\tname:            _params.containerName\n\t_baseContainers: context.output.spec.template.spec.containers\n\t_matchContainers_: [ for _container_ in _baseContainers if _container_.name == name {_container_}]\n\t_baseContainer: *_|_ | {...}\n\tif len(_matchContainers_) == 0 {\n\t\terr: \"container \\(name) not found\"\n\t}\n\tif len(_matchContainers_) \u003e 0 {\n\t\t// +patchStrategy=retainKeys\n\t\timage: _params.image\n\n\t\tif _params.imagePullPolicy != \"\" {\n\t\t\t// +patchStrategy=retainKeys\n\t\t\timagePullPolicy: _params.imagePullPolicy\n\t\t}\n\t}\n}\npatch: spec: template: spec: {\n\tif parameter.containers == _|_ {\n\t\t// +patchKey=name\n\t\tcontainers: [{\n\t\t\tPatchContainer \u0026 {_params: {\n\t\t\t\tif parameter.containerName == \"\" {\n\t\t\t\t\tcontainerName: context.name\n\t\t\t\t}\n\t\t\t\tif parameter.containerName != \"\" {\n\t\t\t\t\tcontainerName: parameter.containerName\n\t\t\t\t}\n\t\t\t\timage:           parameter.image\n\t\t\t\timagePullPolicy: parameter.imagePullPolicy\n\t\t\t}}\n\t\t}]\n\t}\n\tif parameter.containers != _|_ {\n\t\t// +patchKey=name\n\t\tcontainers: [ for c in parameter.containers {\n\t\t\tif c.containerName == \"\" {\n\t\t\t\terr: \"containerName must be set for containers\"\n\t\t\t}\n\t\t\tif c.containerName != \"\" {\n\t\t\t\tPatchContainer \u0026 {_params: c}\n\t\t\t}\n\t\t}]\n\t}\n}\nparameter: *#PatchParams | close({\n\t// +usage=Specify the container image for multiple containers\n\tcontainers: [...#PatchParams]\n})\nerrs: [ for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]\n","parameters":[{"name":"containerName","default":"","usage":"Specify the name of the target container, if not set, use the component name","type":16},{"name":"image","required":true,"default":"","usage":"Specify the image of the container","type":16},{"name":"imagePullPolicy","default":"","usage":"Specify the image pull policy of the container","type":16}],"description":"Set the image of the container.","category":"cue","appliesTo":["deployments.apps","statefulsets.apps","daemonsets.apps","jobs.batch"],"kubetemplate":null}`
	_ = json.Unmarshal([]byte(legacyJSON), &CapContainerImage)
}
