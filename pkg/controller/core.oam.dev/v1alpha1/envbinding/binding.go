/*
 Copyright 2021. The KubeVela Authors.

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
	"context"
	"encoding/json"

	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// EnvBindApp describes the app bound to the environment
type EnvBindApp struct {
	baseApp    *v1beta1.Application
	PatchedApp *v1beta1.Application
	envConfig  *v1alpha1.EnvConfig

	componentManifests []*types.ComponentManifest
	assembledManifests map[string][]*unstructured.Unstructured

	ScheduledManifests map[string]*unstructured.Unstructured
}

// NewEnvBindApp create EnvBindApp
func NewEnvBindApp(base *v1beta1.Application, envConfig *v1alpha1.EnvConfig) *EnvBindApp {
	return &EnvBindApp{
		baseApp:   base,
		envConfig: envConfig,
	}
}

// GenerateConfiguredApplication patch component parameters to base Application
func (e *EnvBindApp) GenerateConfiguredApplication() error {
	newApp := e.baseApp.DeepCopy()

	var baseComponent *common.ApplicationComponent
	var misMatchedIdxs []int
	for patchIdx := range e.envConfig.Patch.Components {
		var matchedIdx int
		isMatched := false
		patchComponent := e.envConfig.Patch.Components[patchIdx]

		for baseIdx := range e.baseApp.Spec.Components {
			component := e.baseApp.Spec.Components[baseIdx]
			if patchComponent.Name == component.Name && patchComponent.Type == component.Type {
				matchedIdx, baseComponent = baseIdx, &component
				isMatched = true
				break
			}
		}
		if !isMatched || baseComponent == nil {
			misMatchedIdxs = append(misMatchedIdxs, patchIdx)
			continue
		}
		targetComponent, err := PatchComponent(baseComponent, &patchComponent)
		if err != nil {
			return err
		}
		newApp.Spec.Components[matchedIdx] = *targetComponent
	}
	for _, idx := range misMatchedIdxs {
		newApp.Spec.Components = append(newApp.Spec.Components, e.envConfig.Patch.Components[idx])
	}
	// select which components to use
	if e.envConfig.Selector != nil {
		compMap := make(map[string]bool)
		if len(e.envConfig.Selector.Components) > 0 {
			for _, comp := range e.envConfig.Selector.Components {
				compMap[comp] = true
			}
		}
		comps := make([]common.ApplicationComponent, 0)
		for _, comp := range newApp.Spec.Components {
			if _, ok := compMap[comp.Name]; ok {
				comps = append(comps, comp)
			}
		}
		newApp.Spec.Components = comps
	}
	e.PatchedApp = newApp
	return nil
}

func (e *EnvBindApp) render(ctx context.Context, appParser *appfile.Parser) error {
	if e.PatchedApp == nil {
		return errors.New("EnvBindApp must has been generated a configured Application")
	}
	ctx = util.SetNamespaceInCtx(ctx, e.PatchedApp.Namespace)
	appFile, err := appParser.GenerateAppFile(ctx, e.PatchedApp)
	if err != nil {
		return err
	}
	comps, err := appFile.GenerateComponentManifests()
	if err != nil {
		return err
	}
	e.componentManifests = comps
	return nil
}

func (e *EnvBindApp) assemble() error {
	if e.componentManifests == nil {
		return errors.New("EnvBindApp must has been rendered")
	}

	assembledManifests := make(map[string][]*unstructured.Unstructured, len(e.componentManifests))
	for _, comp := range e.componentManifests {
		resources := make([]*unstructured.Unstructured, len(comp.Traits)+1)
		workload := comp.StandardWorkload
		workload.SetName(comp.Name)
		e.SetNamespace(workload)
		util.AddLabels(workload, map[string]string{oam.LabelOAMResourceType: oam.ResourceTypeWorkload})
		resources[0] = workload

		for i := 0; i < len(comp.Traits); i++ {
			trait := comp.Traits[i]
			util.AddLabels(trait, map[string]string{oam.LabelOAMResourceType: oam.ResourceTypeTrait})
			e.SetTraitName(comp.Name, trait)
			e.SetNamespace(trait)
			resources[i+1] = trait
		}
		assembledManifests[comp.Name] = resources

		if len(comp.PackagedWorkloadResources) != 0 {
			assembledManifests[comp.Name] = append(assembledManifests[comp.Name], comp.PackagedWorkloadResources...)
		}
	}
	e.assembledManifests = assembledManifests
	return nil
}

// SetTraitName set name for Trait
func (e *EnvBindApp) SetTraitName(compName string, trait *unstructured.Unstructured) {
	if len(trait.GetName()) == 0 {
		traitType := trait.GetLabels()[oam.TraitTypeLabel]
		traitName := util.GenTraitNameCompatible(compName, trait, traitType)
		trait.SetName(traitName)
	}
}

// SetNamespace set namespace for *unstructured.Unstructured
func (e *EnvBindApp) SetNamespace(resource *unstructured.Unstructured) {
	if len(resource.GetNamespace()) != 0 {
		return
	}
	appNs := e.PatchedApp.Namespace
	if len(appNs) == 0 {
		appNs = "default"
	}
	resource.SetNamespace(appNs)
}

// CreateEnvBindApps create EnvBindApps from different envs
func CreateEnvBindApps(envBinding *v1alpha1.EnvBinding, baseApp *v1beta1.Application) ([]*EnvBindApp, error) {
	envBindApps := make([]*EnvBindApp, len(envBinding.Spec.Envs))
	for i := range envBinding.Spec.Envs {
		env := envBinding.Spec.Envs[i]
		envBindApp := NewEnvBindApp(baseApp, &env)
		err := envBindApp.GenerateConfiguredApplication()
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to patch parameter for env %s", env.Name)
		}
		envBindApps[i] = envBindApp
	}
	return envBindApps, nil
}

// RenderEnvBindApps render EnvBindApps
func RenderEnvBindApps(ctx context.Context, envBindApps []*EnvBindApp, appParser *appfile.Parser) error {
	for _, envBindApp := range envBindApps {
		err := envBindApp.render(ctx, appParser)
		if err != nil {
			return errors.WithMessagef(err, "fail to render application for env %s", envBindApp.envConfig.Name)
		}
	}
	return nil
}

// AssembleEnvBindApps assemble resource for EnvBindApp
func AssembleEnvBindApps(envBindApps []*EnvBindApp) error {
	for _, envBindApp := range envBindApps {
		err := envBindApp.assemble()
		if err != nil {
			return errors.WithMessagef(err, "fail to assemble resource for application in env %s", envBindApp.envConfig.Name)
		}
	}
	return nil
}

// PatchComponent patch component parameter to target component parameter
func PatchComponent(baseComponent *common.ApplicationComponent, patchComponent *common.ApplicationComponent) (*common.ApplicationComponent, error) {
	targetComponent := baseComponent.DeepCopy()

	mergedProperties, err := PatchProperties(baseComponent.Properties, patchComponent.Properties)
	if err != nil {
		return nil, errors.WithMessagef(err, "fail to patch properties for component %s", baseComponent.Name)
	}
	targetComponent.Properties = util.Object2RawExtension(mergedProperties)

	var baseTrait *common.ApplicationTrait
	var misMatchedIdxs []int
	for patchIdx := range patchComponent.Traits {
		var matchedIdx int
		isMatched := false
		patchTrait := patchComponent.Traits[patchIdx]

		for index := range targetComponent.Traits {
			trait := targetComponent.Traits[index]
			if patchTrait.Type == trait.Type {
				matchedIdx, baseTrait = index, &trait
				isMatched = true
				break
			}
		}
		if !isMatched || baseTrait == nil {
			misMatchedIdxs = append(misMatchedIdxs, patchIdx)
			continue
		}
		mergedProperties, err = PatchProperties(baseTrait.Properties, patchTrait.Properties)
		if err != nil {
			return nil, err
		}
		targetComponent.Traits[matchedIdx].Properties = util.Object2RawExtension(mergedProperties)
	}

	for _, idx := range misMatchedIdxs {
		targetComponent.Traits = append(targetComponent.Traits, patchComponent.Traits[idx])
	}
	return targetComponent, nil
}

// PatchProperties merge patch parameter for dst parameter
func PatchProperties(dst *runtime.RawExtension, patch *runtime.RawExtension) (map[string]interface{}, error) {
	patchParameter, err := util.RawExtension2Map(patch)
	if err != nil {
		return nil, err
	}
	baseParameter, err := util.RawExtension2Map(dst)
	if err != nil {
		return nil, err
	}
	if baseParameter == nil {
		baseParameter = make(map[string]interface{})
	}
	opts := []func(*mergo.Config){
		// WithOverride will make merge override non-empty dst attributes with non-empty src attributes values.
		mergo.WithOverride,
	}
	err = mergo.Merge(&baseParameter, patchParameter, opts...)
	if err != nil {
		return nil, err
	}
	return baseParameter, nil
}

// StoreManifest2ConfigMap store manifest to configmap
func StoreManifest2ConfigMap(ctx context.Context, cli client.Client, envBinding *v1alpha1.EnvBinding, apps []*EnvBindApp) error {
	cm := new(corev1.ConfigMap)
	data := make(map[string]string)
	for _, app := range apps {
		m := make(map[string]map[string]interface{})
		for name, manifest := range app.ScheduledManifests {
			m[name] = manifest.UnstructuredContent()
		}
		d, err := json.Marshal(m)
		if err != nil {
			return errors.Wrapf(err, "fail to marshal patched application for env %s", app.envConfig.Name)
		}
		data[app.envConfig.Name] = string(d)
	}
	cm.Data = data
	cm.SetName(envBinding.Spec.OutputResourcesTo.Name)
	if len(envBinding.Spec.OutputResourcesTo.Namespace) == 0 {
		cm.SetNamespace("default")
	} else {
		cm.SetNamespace(envBinding.Spec.OutputResourcesTo.Namespace)
	}

	ownerReference := []metav1.OwnerReference{{
		APIVersion:         envBinding.APIVersion,
		Kind:               envBinding.Kind,
		Name:               envBinding.Name,
		UID:                envBinding.GetUID(),
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}}
	cm.SetOwnerReferences(ownerReference)

	cmCopy := cm.DeepCopy()
	if err := cli.Get(ctx, client.ObjectKey{Namespace: cmCopy.Namespace, Name: cmCopy.Name}, cmCopy); err != nil {
		if kerrors.IsNotFound(err) {
			return cli.Create(ctx, cm)
		}
		return err
	}
	return cli.Update(ctx, cm)
}
