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

package envbinding

import (
	"encoding/json"
	"fmt"

	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	errors2 "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// MergeRawExtension merge two raw extension
func MergeRawExtension(base *runtime.RawExtension, patch *runtime.RawExtension) (*runtime.RawExtension, error) {
	patchParameter, err := util.RawExtension2Map(patch)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert patch parameters to map")
	}
	baseParameter, err := util.RawExtension2Map(base)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert base parameters to map")
	}
	if baseParameter == nil {
		baseParameter = make(map[string]interface{})
	}
	err = mergo.Merge(&baseParameter, patchParameter, mergo.WithOverride)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to do merge with override")
	}
	bs, err := json.Marshal(baseParameter)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal merged properties")
	}
	return &runtime.RawExtension{Raw: bs}, nil
}

// MergeComponent merge two component, it will first merge their properties and then merge their traits
func MergeComponent(base *common.ApplicationComponent, patch *common.ApplicationComponent) (*common.ApplicationComponent, error) {
	newComponent := base.DeepCopy()
	var err error

	// merge component properties
	newComponent.Properties, err = MergeRawExtension(base.Properties, patch.Properties)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to merge component properties")
	}

	// prepare traits
	traitMaps := map[string]*common.ApplicationTrait{}
	var traitOrders []string
	for _, trait := range base.Traits {
		traitMaps[trait.Type] = trait.DeepCopy()
		traitOrders = append(traitOrders, trait.Type)
	}

	// patch traits
	var errs errors2.ErrorList
	for _, trait := range patch.Traits {
		if baseTrait, exists := traitMaps[trait.Type]; exists {
			baseTrait.Properties, err = MergeRawExtension(baseTrait.Properties, trait.Properties)
			if err != nil {
				errs.Append(errors.Wrapf(err, "failed to merge trait %s", trait.Type))
			}
		} else {
			traitMaps[trait.Type] = trait.DeepCopy()
			traitOrders = append(traitOrders, trait.Type)
		}
	}
	if errs.HasError() {
		return nil, errors.Wrapf(err, "failed to merge component traits")
	}

	// fill in traits
	newComponent.Traits = []common.ApplicationTrait{}
	for _, traitType := range traitOrders {
		newComponent.Traits = append(newComponent.Traits, *traitMaps[traitType])
	}
	return newComponent, nil
}

func filterComponents(components []string, selector *v1alpha1.EnvSelector) []string {
	if selector != nil && len(selector.Components) > 0 {
		filter := map[string]bool{}
		for _, compName := range selector.Components {
			filter[compName] = true
		}
		var _comps []string
		for _, compName := range components {
			if _, ok := filter[compName]; ok {
				_comps = append(_comps, compName)
			}
		}
		return _comps
	}
	return components
}

// PatchApplication patch base application with patch and selector
func PatchApplication(base *v1beta1.Application, patch *v1alpha1.EnvPatch, selector *v1alpha1.EnvSelector) (*v1beta1.Application, error) {
	newApp := base.DeepCopy()

	// init components
	compMaps := map[string]*common.ApplicationComponent{}
	var compOrders []string
	for _, comp := range base.Spec.Components {
		compMaps[comp.Name] = comp.DeepCopy()
		compOrders = append(compOrders, comp.Name)
	}

	// patch components
	var errs errors2.ErrorList
	var err error
	for _, comp := range patch.Components {
		if baseComp, exists := compMaps[comp.Name]; exists {
			if baseComp.Type != comp.Type {
				compMaps[comp.Name] = comp.DeepCopy()
			} else {
				compMaps[comp.Name], err = MergeComponent(baseComp, comp.DeepCopy())
				if err != nil {
					errs.Append(errors.Wrapf(err, "failed to merge component %s", comp.Name))
				}
			}
		} else {
			compMaps[comp.Name] = comp.DeepCopy()
			compOrders = append(compOrders, comp.Name)
		}
	}
	if errs.HasError() {
		return nil, errors.Wrapf(err, "failed to merge application components")
	}
	newApp.Spec.Components = []common.ApplicationComponent{}

	// if selector is enabled, filter
	compOrders = filterComponents(compOrders, selector)

	// fill in new application
	for _, compName := range compOrders {
		newApp.Spec.Components = append(newApp.Spec.Components, *compMaps[compName])
	}
	return newApp, nil
}

// PatchApplicationByEnvBindingEnv get patched application directly through policyName and envName
func PatchApplicationByEnvBindingEnv(app *v1beta1.Application, policyName string, envName string) (*v1beta1.Application, error) {
	policy, err := GetEnvBindingPolicy(app, policyName)
	if err != nil {
		return nil, err
	}
	if policy != nil {
		for _, env := range policy.Envs {
			if env.Name == envName {
				return PatchApplication(app, &env.Patch, env.Selector)
			}
		}
	}
	return nil, fmt.Errorf("target env %s in policy %s not found", envName, policyName)
}
