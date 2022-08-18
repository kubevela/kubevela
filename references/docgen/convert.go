/*
 Copyright 2022 The KubeVela Authors.

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

package docgen

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// ParseCapabilityFromUnstructured will convert Unstructured to Capability
func ParseCapabilityFromUnstructured(mapper discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover, obj unstructured.Unstructured) (types.Capability, error) {
	var err error
	switch obj.GetKind() {
	case "ComponentDefinition":
		var cd v1beta1.ComponentDefinition
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cd)
		if err != nil {
			return types.Capability{}, err
		}
		var workloadDefinitionRef string
		if cd.Spec.Workload.Type != "" {
			workloadDefinitionRef = cd.Spec.Workload.Type
		} else if mapper != nil {
			ref, err := util.ConvertWorkloadGVK2Definition(mapper, cd.Spec.Workload.Definition)
			if err != nil {
				return types.Capability{}, err
			}
			workloadDefinitionRef = ref.Name
		}
		return HandleDefinition(cd.Name, workloadDefinitionRef, cd.Annotations, cd.Labels, cd.Spec.Extension, types.TypeComponentDefinition, nil, cd.Spec.Schematic, pd)
	case "TraitDefinition":
		var td v1beta1.TraitDefinition
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &td)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(td.Name, td.Spec.Reference.Name, td.Annotations, td.Labels, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads, td.Spec.Schematic, pd)
	case "PolicyDefinition":
		var plcd v1beta1.PolicyDefinition
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &plcd)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(plcd.Name, plcd.Spec.Reference.Name, plcd.Annotations, plcd.Labels, nil, types.TypePolicy, nil, plcd.Spec.Schematic, pd)
	case "WorkflowStepDefinition":
		var wfd v1beta1.WorkflowStepDefinition
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &wfd)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(wfd.Name, wfd.Spec.Reference.Name, wfd.Annotations, wfd.Labels, nil, types.TypeWorkflowStep, nil, wfd.Spec.Schematic, pd)
	}
	return types.Capability{}, fmt.Errorf("unknown definition Type %s", obj.GetKind())
}
