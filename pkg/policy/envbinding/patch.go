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
	"regexp"
	"strings"

	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/policy/utils"
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
func MergeComponent(base *common.ApplicationComponent, patch *v1alpha1.EnvComponentPatch) (*common.ApplicationComponent, error) {
	newComponent := base.DeepCopy()
	var err error

	// merge component properties
	newComponent.Properties, err = MergeRawExtension(base.Properties, patch.Properties)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to merge component properties")
	}

	// merge component external revision
	if patch.ExternalRevision != "" {
		newComponent.ExternalRevision = patch.ExternalRevision
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
			if trait.Disable {
				delete(traitMaps, trait.Type)
				continue
			}
			baseTrait.Properties, err = MergeRawExtension(baseTrait.Properties, trait.Properties)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to merge trait %s", trait.Type))
			}
		} else {
			if trait.Disable {
				continue
			}
			traitMaps[trait.Type] = trait.ToApplicationTrait()
			traitOrders = append(traitOrders, trait.Type)
		}
	}
	if errs.HasError() {
		return nil, errors.Wrapf(err, "failed to merge component traits")
	}

	// fill in traits
	newComponent.Traits = []common.ApplicationTrait{}
	for _, traitType := range traitOrders {
		if _, exists := traitMaps[traitType]; exists {
			newComponent.Traits = append(newComponent.Traits, *traitMaps[traitType])
		}
	}
	return newComponent, nil
}

// PatchApplication patch base application with patch and selector
func PatchApplication(base *v1beta1.Application, patch *v1alpha1.EnvPatch, selector *v1alpha1.EnvSelector) (*v1beta1.Application, error) {
	newApp := base.DeepCopy()
	var err error
	var compSelector []string
	if selector != nil {
		compSelector = selector.Components
	}
	var compPatch []v1alpha1.EnvComponentPatch
	if patch != nil {
		compPatch = patch.Components
	}
	newApp.Spec.Components, err = PatchComponents(base.Spec.Components, compPatch, compSelector)
	return newApp, err
}

// PatchComponents patch base components with patch and selector
func PatchComponents(baseComponents []common.ApplicationComponent, patchComponents []v1alpha1.EnvComponentPatch, selector []string) ([]common.ApplicationComponent, error) {
	// init components
	compMaps := map[string]*common.ApplicationComponent{}
	var compOrders []string
	for _, comp := range baseComponents {
		compMaps[comp.Name] = comp.DeepCopy()
		compOrders = append(compOrders, comp.Name)
	}

	// patch components
	var errs errors2.ErrorList
	var err error
	for _, comp := range patchComponents {
		if comp.Name == "" {
			// when no component name specified in the patch
			// 1. if no type name specified in the patch, it will merge all components
			// 2. if type name specified, it will merge components with the specified type
			for compName, baseComp := range compMaps {
				if comp.Type == "" || comp.Type == baseComp.Type {
					compMaps[compName], err = MergeComponent(baseComp, comp.DeepCopy())
					if err != nil {
						errs = append(errs, errors.Wrapf(err, "failed to merge component %s", compName))
					}
				}
			}
		} else {
			// when component name (pattern) specified in the patch, it will find the component with the matched name
			// 1. if the component type is not specified in the patch, the matched component will be merged with the patch
			// 2. if the matched component uses the same type, the matched component will be merged with the patch
			// 3. if the matched component uses a different type, the matched component will be overridden by the patch
			// 4. if no component matches, and the component name is a valid kubernetes name, a new component will be added
			addComponent := regexp.MustCompile("[a-z]([a-z-]{0,61}[a-z])?").MatchString(comp.Name)
			if re, err := regexp.Compile(strings.ReplaceAll(comp.Name, "*", ".*")); err == nil {
				for compName, baseComp := range compMaps {
					if re.MatchString(compName) {
						addComponent = false
						if baseComp.Type != comp.Type && comp.Type != "" {
							compMaps[compName] = comp.ToApplicationComponent()
						} else {
							compMaps[compName], err = MergeComponent(baseComp, comp.DeepCopy())
							if err != nil {
								errs = append(errs, errors.Wrapf(err, "failed to merge component %s", comp.Name))
							}
						}
					}
				}
			}
			if addComponent {
				compMaps[comp.Name] = comp.ToApplicationComponent()
				compOrders = append(compOrders, comp.Name)
			}
		}
	}
	if errs.HasError() {
		return nil, errors.Wrapf(err, "failed to merge application components")
	}

	// if selector is enabled, filter
	compOrders = utils.FilterComponents(compOrders, selector)

	// fill in new application
	newComponents := []common.ApplicationComponent{}
	for _, compName := range compOrders {
		newComponents = append(newComponents, *compMaps[compName])
	}
	return newComponents, nil
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
