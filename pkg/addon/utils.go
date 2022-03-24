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

package addon

import (
	"context"
	"fmt"
	"strings"

	errors "github.com/pkg/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	rest "k8s.io/client-go/rest"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	compDefAnnotation         = "addon.oam.dev/componentDefinitions"
	traitDefAnnotation        = "addon.oam.dev/traitDefinitions"
	workflowStepDefAnnotation = "addon.oam.dev/workflowStepDefinitions"
	defKeytemplate            = "addon-%s-%s"
)

// parse addon's created x-defs in addon-app's annotation, this will be used to check whether app still using it while disabling.
func passDefInAppAnnotation(defs []*unstructured.Unstructured, app *v1beta1.Application) error {
	var comps, traits, workflowSteps []string
	for _, def := range defs {
		switch def.GetObjectKind().GroupVersionKind().Kind {
		case v1beta1.ComponentDefinitionKind:
			comps = append(comps, def.GetName())
		case v1beta1.TraitDefinitionKind:
			traits = append(traits, def.GetName())
		case v1beta1.WorkflowStepDefinitionKind:
			workflowSteps = append(workflowSteps, def.GetName())
		default:
			return fmt.Errorf("cannot handle definition types %s, name %s", def.GetObjectKind().GroupVersionKind().Kind, def.GetName())
		}
	}
	if len(comps) != 0 {
		app.SetAnnotations(util.MergeMapOverrideWithDst(app.GetAnnotations(), map[string]string{compDefAnnotation: strings.Join(comps, ",")}))
	}
	if len(traits) != 0 {
		app.SetAnnotations(util.MergeMapOverrideWithDst(app.GetAnnotations(), map[string]string{traitDefAnnotation: strings.Join(traits, ",")}))
	}
	if len(workflowSteps) != 0 {
		app.SetAnnotations(util.MergeMapOverrideWithDst(app.GetAnnotations(), map[string]string{workflowStepDefAnnotation: strings.Join(workflowSteps, ",")}))
	}
	return nil
}

// check whether this addon has been used by some applications
func checkAddonHasBeenUsed(ctx context.Context, k8sClient client.Client, name string, addonApp v1beta1.Application, config *rest.Config) ([]v1beta1.Application, error) {
	apps := v1beta1.ApplicationList{}
	if err := k8sClient.List(ctx, &apps, client.InNamespace("")); err != nil {
		return nil, err
	}

	if len(apps.Items) == 0 {
		return nil, nil
	}

	createdDefs := make(map[string]bool)
	for key, defNames := range addonApp.GetAnnotations() {
		switch key {
		case compDefAnnotation, traitDefAnnotation, workflowStepDefAnnotation:
			merge2DefMap(key, defNames, createdDefs)
		}
	}

	if len(createdDefs) == 0 {
		if err := findLegacyAddonDefs(ctx, k8sClient, name, addonApp.GetLabels()[oam.LabelAddonRegistry], config, createdDefs); err != nil {
			return nil, err
		}
	}

	var res []v1beta1.Application
CHECKNEXT:
	for _, app := range apps.Items {
		for _, component := range app.Spec.Components {
			if createdDefs[fmt.Sprintf(defKeytemplate, "comp", component.Type)] {
				res = append(res, app)
				// this app has used this addon, there is no need check other components
				continue CHECKNEXT
			}
			for _, trait := range component.Traits {
				if createdDefs[fmt.Sprintf(defKeytemplate, "trait", trait.Type)] {
					res = append(res, app)
					continue CHECKNEXT
				}
			}
		}
		if app.Spec.Workflow == nil || len(app.Spec.Workflow.Steps) == 0 {
			return res, nil
		}
		for _, s := range app.Spec.Workflow.Steps {
			if createdDefs[fmt.Sprintf(defKeytemplate, "wfStep", s.Type)] {
				res = append(res, app)
				continue CHECKNEXT
			}
		}
	}
	return res, nil
}

//  merge2DefMap will parse annotation in addon's app to 'created x-definition'. Then stroe them in defMap
func merge2DefMap(defType string, defNames string, defMap map[string]bool) {
	list := strings.Split(defNames, ",")
	template := "addon-%s-%s"
	for _, defName := range list {
		switch defType {
		case compDefAnnotation:
			defMap[fmt.Sprintf(template, "comp", defName)] = true
		case traitDefAnnotation:
			defMap[fmt.Sprintf(template, "trait", defName)] = true
		case workflowStepDefAnnotation:
			defMap[fmt.Sprintf(template, "wfStep", defName)] = true
		}
	}
}

// for old addon's app no 'created x-definitions' annotation, fetch the definitions from alive addon registry. Put them in defMap
func findLegacyAddonDefs(ctx context.Context, k8sClient client.Client, addonName string, registryName string, config *rest.Config, defs map[string]bool) error {
	// if the addon enable by local we cannot fetch the source definitions yet, so skip the check
	if registryName == "local" {
		return nil
	}

	registryDS := NewRegistryDataStore(k8sClient)
	registries, err := registryDS.ListRegistries(ctx)
	if err != nil {
		return err
	}
	var defObjects []*unstructured.Unstructured
	for i, registry := range registries {
		if registry.Name == registryName {
			var uiData *UIData
			if !IsVersionRegistry(registry) {
				installer := NewAddonInstaller(ctx, k8sClient, nil, nil, config, &registries[i], nil, nil)
				metas, err := installer.getAddonMeta()
				if err != nil {
					return err
				}
				meta := metas[addonName]
				// only fetch definition files from registry.
				uiData, err = registry.GetUIData(&meta, UnInstallOptions)
				if err != nil {
					return errors.Wrapf(err, "cannot fetch addon difinition files from registry")
				}
			} else {
				versionedRegistry := BuildVersionedRegistry(registry.Name, registry.Helm.URL)
				uiData, err = versionedRegistry.GetAddonUIData(ctx, addonName, "")
				if err != nil {
					return errors.Wrapf(err, "cannot fetch addon difinition files from registry")
				}
			}

			for _, defYaml := range uiData.Definitions {
				def, err := renderObject(defYaml)
				if err != nil {
					// don't let one error defined definition block whole disable process
					continue
				}
				defObjects = append(defObjects, def)
			}
			for _, cueDef := range uiData.CUEDefinitions {
				def := definition.Definition{Unstructured: unstructured.Unstructured{}}
				err := def.FromCUEString(cueDef.Data, config)
				if err != nil {
					// don't let one error defined cue definition block whole disable process
					continue
				}
				defObjects = append(defObjects, &def.Unstructured)
			}
		}
	}
	for _, defObject := range defObjects {
		switch defObject.GetObjectKind().GroupVersionKind().Kind {
		case v1beta1.ComponentDefinitionKind:
			defs[fmt.Sprintf(defKeytemplate, "comp", defObject.GetName())] = true
		case v1beta1.TraitDefinitionKind:
			defs[fmt.Sprintf(defKeytemplate, "trait", defObject.GetName())] = true
		case v1beta1.WorkflowStepDefinitionKind:
			defs[fmt.Sprintf(defKeytemplate, "wfStep", defObject.GetName())] = true
		}
	}
	return nil
}

func usingAppsInfo(apps []v1beta1.Application) string {
	res := "addon is being used :"
	appsNamespaceNameList := map[string][]string{}
	for _, app := range apps {
		appsNamespaceNameList[app.GetNamespace()] = append(appsNamespaceNameList[app.GetNamespace()], app.GetName())
	}
	for namespace, appNames := range appsNamespaceNameList {
		nameStr := strings.Join(appNames, ",")
		res += fmt.Sprintf("{%s} in namespace:%s,", nameStr, namespace)
	}
	res = strings.TrimSuffix(res, ",") + ".Please delete them before disabling the addon."
	return res
}

func IsVersionRegistry(r Registry) bool {
	if r.Helm != nil {
		return true
	}
	return false
}
