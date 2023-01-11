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
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	compDefAnnotation         = "addon.oam.dev/componentDefinitions"
	traitDefAnnotation        = "addon.oam.dev/traitDefinitions"
	workflowStepDefAnnotation = "addon.oam.dev/workflowStepDefinitions"
	policyDefAnnotation       = "addon.oam.dev/policyDefinitions"
	defKeytemplate            = "addon-%s-%s"
	compMapKey                = "comp"
	traitMapKey               = "trait"
	wfStepMapKey              = "wfStep"
	policyMapKey              = "policy"
)

// parse addon's created x-defs in addon-app's annotation, this will be used to check whether app still using it while disabling.
func passDefInAppAnnotation(defs []*unstructured.Unstructured, app *v1beta1.Application) error {
	var comps, traits, workflowSteps, policies []string
	for _, def := range defs {
		if !checkBondComponentExist(*def, *app) {
			// if the definition binding a component, and the component not exist, skip recording.
			continue
		}
		switch def.GetObjectKind().GroupVersionKind().Kind {
		case v1beta1.ComponentDefinitionKind:
			comps = append(comps, def.GetName())
		case v1beta1.TraitDefinitionKind:
			traits = append(traits, def.GetName())
		case v1beta1.WorkflowStepDefinitionKind:
			workflowSteps = append(workflowSteps, def.GetName())
		case v1beta1.PolicyDefinitionKind:
			policies = append(policies, def.GetName())
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
	if len(policies) != 0 {
		app.SetAnnotations(util.MergeMapOverrideWithDst(app.GetAnnotations(), map[string]string{policyDefAnnotation: strings.Join(policies, ",")}))
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
		case compDefAnnotation, traitDefAnnotation, workflowStepDefAnnotation, policyDefAnnotation:
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
			if createdDefs[fmt.Sprintf(defKeytemplate, compMapKey, component.Type)] {
				res = append(res, app)
				// this app has used this addon, there is no need check other components
				continue CHECKNEXT
			}
			for _, trait := range component.Traits {
				if createdDefs[fmt.Sprintf(defKeytemplate, traitMapKey, trait.Type)] {
					res = append(res, app)
					continue CHECKNEXT
				}
			}
		}

		if app.Spec.Workflow != nil && len(app.Spec.Workflow.Steps) != 0 {
			for _, s := range app.Spec.Workflow.Steps {
				if createdDefs[fmt.Sprintf(defKeytemplate, wfStepMapKey, s.Type)] {
					res = append(res, app)
					continue CHECKNEXT
				}
			}
		}

		if app.Spec.Policies != nil && len(app.Spec.Policies) != 0 {
			for _, p := range app.Spec.Policies {
				if createdDefs[fmt.Sprintf(defKeytemplate, policyMapKey, p.Type)] {
					res = append(res, app)
					continue CHECKNEXT
				}
			}
		}
	}
	return res, nil
}

// merge2DefMap will parse annotation in addon's app to 'created x-definition'. Then stroe them in defMap
func merge2DefMap(defType string, defNames string, defMap map[string]bool) {
	list := strings.Split(defNames, ",")
	template := "addon-%s-%s"
	for _, defName := range list {
		switch defType {
		case compDefAnnotation:
			defMap[fmt.Sprintf(template, compMapKey, defName)] = true
		case traitDefAnnotation:
			defMap[fmt.Sprintf(template, traitMapKey, defName)] = true
		case workflowStepDefAnnotation:
			defMap[fmt.Sprintf(template, wfStepMapKey, defName)] = true
		case policyDefAnnotation:
			defMap[fmt.Sprintf(template, policyMapKey, defName)] = true
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
				installer := NewAddonInstaller(ctx, k8sClient, nil, nil, config, &registries[i], nil, nil, nil)
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
				versionedRegistry := BuildVersionedRegistry(registry.Name, registry.Helm.URL, &common.HTTPOption{
					Username:        registry.Helm.Username,
					Password:        registry.Helm.Password,
					InsecureSkipTLS: registry.Helm.InsecureSkipTLS,
				})
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
		case v1beta1.PolicyDefinitionKind:

		}
	}
	return nil
}

func appsDependsOnAddonErrInfo(apps []v1beta1.Application) string {
	var appsNamespaceNameList []string
	i := 0
	for _, app := range apps {
		appsNamespaceNameList = append(appsNamespaceNameList, app.Namespace+"/"+app.Name)
		i++
		if i > 2 && len(apps) > i {
			appsNamespaceNameList = append(appsNamespaceNameList, fmt.Sprintf("and other %d more", len(apps)-i))
			break
		}
	}
	return fmt.Sprintf("this addon is being used by: %s applications. Please delete all of them before removing.", strings.Join(appsNamespaceNameList, ", "))
}

// IsVersionRegistry  check the repo source if support multi-version addon
func IsVersionRegistry(r Registry) bool {
	return r.Helm != nil
}

// InstallOption define additional option for installation
type InstallOption func(installer *Installer)

// SkipValidateVersion means skip validating system version
func SkipValidateVersion(installer *Installer) {
	installer.skipVersionValidate = true
}

// DryRunAddon means only generate yaml for addon instead of installing it
func DryRunAddon(installer *Installer) {
	installer.dryRun = true
}

// OverrideDefinitions means override definitions within this addon if some of them already exist
func OverrideDefinitions(installer *Installer) {
	installer.overrideDefs = true
}

// IsAddonDir validates an addon directory.
// It checks required files like metadata.yaml and template.yaml
func IsAddonDir(dirName string) (bool, error) {
	if fi, err := os.Stat(dirName); err != nil {
		return false, err
	} else if !fi.IsDir() {
		return false, errors.Errorf("%q is not a directory", dirName)
	}

	// Load metadata.yaml
	metadataYaml := filepath.Join(dirName, MetadataFileName)
	if _, err := os.Stat(metadataYaml); os.IsNotExist(err) {
		return false, errors.Errorf("no %s exists in directory %q", MetadataFileName, dirName)
	}
	metadataYamlContent, err := os.ReadFile(filepath.Clean(metadataYaml))
	if err != nil {
		return false, errors.Errorf("cannot read %s in directory %q", MetadataFileName, dirName)
	}

	// Check metadata.yaml contents
	metadataContent := new(Meta)
	if err := yaml.Unmarshal(metadataYamlContent, &metadataContent); err != nil {
		return false, err
	}
	if metadataContent == nil {
		return false, errors.Errorf("metadata (%s) missing", MetadataFileName)
	}
	if metadataContent.Name == "" {
		return false, errors.Errorf("addon name is empty")
	}
	if metadataContent.Version == "" {
		return false, errors.Errorf("addon version is empty")
	}

	// Load template.yaml/cue
	var errYAML error
	var errCUE error
	templateYAML := filepath.Join(dirName, TemplateFileName)
	templateCUE := filepath.Join(dirName, AppTemplateCueFileName)
	_, errYAML = os.Stat(templateYAML)
	_, errCUE = os.Stat(templateCUE)
	if os.IsNotExist(errYAML) && os.IsNotExist(errCUE) {
		return false, fmt.Errorf("no %s or %s exists in directory %q", TemplateFileName, AppTemplateCueFileName, dirName)
	}
	if errYAML != nil && errCUE != nil {
		return false, errors.Errorf("cannot stat %s or %s", TemplateFileName, AppTemplateCueFileName)
	}

	// template.cue have higher priority
	if errCUE == nil {
		templateContent, err := os.ReadFile(filepath.Clean(templateCUE))
		if err != nil {
			return false, fmt.Errorf("cannot read %s: %w", AppTemplateCueFileName, err)
		}
		// Just look for `output` field is enough.
		// No need to load the whole addon package to render the Application.
		if !strings.Contains(string(templateContent), renderOutputCuePath) {
			return false, fmt.Errorf("no %s field in %s", renderOutputCuePath, AppTemplateCueFileName)
		}
		return true, nil
	}

	// then check template.yaml
	templateYamlContent, err := os.ReadFile(filepath.Clean(templateYAML))
	if err != nil {
		return false, errors.Errorf("cannot read %s in directory %q", TemplateFileName, dirName)
	}
	// Check template.yaml contents
	template := new(v1beta1.Application)
	if err := yaml.Unmarshal(templateYamlContent, &template); err != nil {
		return false, err
	}
	if template == nil {
		return false, errors.Errorf("template (%s) missing", TemplateFileName)
	}

	return true, nil
}

// MakeChartCompatible makes an addon directory compatible with Helm Charts.
// It essentially creates a Chart.yaml file in it (if it doesn't already have one).
// If overwrite is true, a Chart.yaml will always be created.
func MakeChartCompatible(addonDir string, overwrite bool) error {
	// Check if it is an addon dir
	isAddonDir, err := IsAddonDir(addonDir)
	if !isAddonDir {
		return fmt.Errorf("%s is not an addon dir: %w", addonDir, err)
	}

	// Check if the addon dir has valid Chart.yaml in it.
	// No need to handle error here.
	// If it doesn't contain a valid Chart.yaml (thus errors), we will create it later.
	isChartDir, _ := chartutil.IsChartDir(addonDir)

	// Only when it is already a Helm Chart, and we don't want to overwrite Chart.yaml,
	// we do nothing.
	if isChartDir && !overwrite {
		return nil
	}

	// Creating Chart.yaml.
	chartMeta, err := generateChartMetadata(addonDir)
	if err != nil {
		return err
	}

	err = chartutil.SaveChartfile(filepath.Join(addonDir, chartutil.ChartfileName), chartMeta)
	if err != nil {
		return err
	}

	return nil
}

// generateChartMetadata generates a Chart.yaml file (chart.Metadata) from an addon metadata file (metadata.yaml).
// It is mostly used to package an addon into a Helm Chart.
func generateChartMetadata(addonDirPath string) (*chart.Metadata, error) {
	// Load addon metadata.yaml
	meta := &Meta{}
	metaData, err := os.ReadFile(filepath.Clean(filepath.Join(addonDirPath, MetadataFileName)))
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(metaData, meta)
	if err != nil {
		return nil, err
	}

	// Generate Chart.yaml from metadata.yaml
	chartMeta := &chart.Metadata{
		Name:        meta.Name,
		Description: meta.Description,
		// Define Vela addon's type to be library in order to prevent installation of a common chart.
		// Please refer to https://helm.sh/docs/topics/library_charts/
		Type:       "library",
		Version:    meta.Version,
		AppVersion: meta.Version,
		APIVersion: chart.APIVersionV2,
		Icon:       meta.Icon,
		Home:       meta.URL,
		Keywords:   meta.Tags,
	}
	annotation := generateAnnotation(meta)
	if len(annotation) != 0 {
		chartMeta.Annotations = annotation
	}
	return chartMeta, nil
}

// generateAnnotation generate addon annotation info for chart.yaml, will recorded in index.yaml in helm repo
func generateAnnotation(meta *Meta) map[string]string {
	res := map[string]string{}
	if meta.SystemRequirements != nil {
		if len(meta.SystemRequirements.VelaVersion) != 0 {
			res[velaSystemRequirement] = meta.SystemRequirements.VelaVersion
		}
		if len(meta.SystemRequirements.KubernetesVersion) != 0 {
			res[kubernetesSystemRequirement] = meta.SystemRequirements.KubernetesVersion
		}
	}
	return res
}

func checkConflictDefs(ctx context.Context, k8sClient client.Client, defs []*unstructured.Unstructured, appName string) (map[string]string, error) {
	res := map[string]string{}
	for _, def := range defs {
		checkDef := def.DeepCopy()
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(checkDef), checkDef)
		if err == nil {
			owner := metav1.GetControllerOf(checkDef)
			if owner == nil || owner.Kind != v1beta1.ApplicationKind {
				res[checkDef.GetName()] = fmt.Sprintf("definition: %s already exist and not belong to any addon \n", checkDef.GetName())
				continue
			}
			if owner.Name != appName {
				// if addon not belong to an addon or addon name is another one, we should put them in result
				res[checkDef.GetName()] = fmt.Sprintf("definition: %s in this addon already exist in %s \n", checkDef.GetName(), addon.AppName2Addon(appName))
			}
		}
		if err != nil && !errors2.IsNotFound(err) {
			return nil, errors.Wrapf(err, "check definition %s", checkDef.GetName())
		}
	}
	return res, nil
}

func produceDefConflictError(conflictDefs map[string]string) error {
	if len(conflictDefs) == 0 {
		return nil
	}
	var errorInfo string
	for _, s := range conflictDefs {
		errorInfo += s
	}
	errorInfo += "if you want override them, please use argument '--override-definitions' to enable \n"
	return errors.New(errorInfo)
}

// checkBondComponentExist will check the ready-to-apply object(def or auxiliary outputs) whether bind to a component
// if the target component not exist, return false.
func checkBondComponentExist(u unstructured.Unstructured, app v1beta1.Application) bool {
	var comp string
	var existKey bool
	comp, existKey = u.GetAnnotations()[oam.AnnotationAddonDefinitionBondCompKey]
	if !existKey {
		// this is compatibility logic for deprecated annotation
		comp, existKey = u.GetAnnotations()[oam.AnnotationIgnoreWithoutCompKey]
		if !existKey {
			// if an object(def or auxiliary outputs ) binding no components return true
			return true
		}
	}
	for _, component := range app.Spec.Components {
		if component.Name == comp {
			// the bond component exists, return ture
			return true
		}
	}
	return false
}

func validateAddonPackage(addonPkg *InstallPackage) error {
	if reflect.DeepEqual(addonPkg.Meta, Meta{}) {
		return fmt.Errorf("the addon package doesn't have `metadata.yaml`")
	}
	if addonPkg.Name == "" {
		return fmt.Errorf("`matadata.yaml` must define the name of addon")
	}
	if addonPkg.Version == "" {
		return fmt.Errorf("`matadata.yaml` must define the version of addon")
	}
	return nil
}

// FilterDependencyRegistries will return all registries besides the target registry itself
func FilterDependencyRegistries(i int, rs []Registry) []Registry {
	if i >= len(rs) {
		return rs
	}
	if i < 0 {
		return rs
	}
	ret := make([]Registry, len(rs)-1)
	copy(ret, rs[:i])
	copy(ret[i:], rs[i+1:])
	return ret
}
