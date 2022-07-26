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

package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"cuelang.org/go/cue/parser"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	cuemodel "github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
)

const (
	specifyAddonClustersTopologyPolicy = "deploy-addon-to-specified-clusters"
	addonAllClusterPolicy              = "deploy-addon-to-all-clusters"
	renderOutputCuePath                = "output"
	renderAuxiliaryOutputsPath         = "outputs"
)

type addonCueTemplateRender struct {
	addon     *InstallPackage
	inputArgs map[string]interface{}
}

func (a addonCueTemplateRender) formatContext() (string, error) {
	args := a.inputArgs
	if args == nil {
		args = map[string]interface{}{}
	}
	bt, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	paramFile := fmt.Sprintf("%s: %s", cuemodel.ParameterFieldName, string(bt))

	var contextFile = strings.Builder{}
	// user custom parameter but be the first data and generated data should be appended at last
	// in case the user defined data has packages
	contextFile.WriteString(a.addon.Parameters + "\n")

	// addon metadata context
	metadataJSON, err := json.Marshal(a.addon.Meta)
	if err != nil {
		return "", err
	}
	contextFile.WriteString(fmt.Sprintf("context: metadata: %s\n", string(metadataJSON)))
	// parameter definition
	contextFile.WriteString(paramFile + "\n")

	return contextFile.String(), nil
}

// This func can be used for addon render component.
// Please notice the result will be stored in object parameter, so object must be a pointer type
func (a addonCueTemplateRender) toObject(cueTemplate string, path string, object interface{}) error {
	contextFile, err := a.formatContext()
	if err != nil {
		return err
	}
	v, err := value.NewValue(contextFile, nil, "")
	if err != nil {
		return err
	}
	out, err := v.LookupByScript(cueTemplate)
	if err != nil {
		return err
	}
	outputContent, err := out.LookupValue(path)
	if err != nil {
		return err
	}
	return outputContent.UnmarshalTo(object)
}

// renderApp will render Application from CUE files
func (a addonCueTemplateRender) renderApp() (*v1beta1.Application, []*unstructured.Unstructured, error) {
	var app v1beta1.Application
	var outputs = map[string]interface{}{}
	var res []*unstructured.Unstructured

	contextFile, err := a.formatContext()
	if err != nil {
		return nil, nil, errors.Wrap(err, "format context for app render")
	}
	contextCue, err := parser.ParseFile("parameter.cue", contextFile, parser.ParseComments)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse parameter context")
	}
	if contextCue.PackageName() == "" {
		contextFile = value.DefaultPackageHeader + contextFile
	}

	var files = []string{contextFile}
	for _, cuef := range a.addon.CUETemplates {
		files = append(files, cuef.Data)
	}

	// TODO(wonderflow): add package discover to support vela own packages if needed
	v, err := value.NewValueWithMainAndFiles(a.addon.AppCueTemplate.Data, files, nil, "")
	if err != nil {
		return nil, nil, errors.Wrap(err, "load app template with CUE files")
	}
	outputContent, err := v.LookupValue(renderOutputCuePath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "render app from output field from CUE")
	}
	err = outputContent.UnmarshalTo(&app)
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode app from CUE")
	}
	auxiliaryContent, err := v.LookupValue(renderAuxiliaryOutputsPath)
	if err != nil {
		// no outputs defined in app template, return normal data
		if isErrorCueRenderPathNotFound(err, renderAuxiliaryOutputsPath) {
			return &app, res, nil
		}
		return nil, nil, errors.Wrap(err, "render app from output field from CUE")
	}

	err = auxiliaryContent.UnmarshalTo(&outputs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode app from CUE")
	}
	for k, o := range outputs {
		if ao, ok := o.(map[string]interface{}); ok {
			auxO := &unstructured.Unstructured{Object: ao}
			auxO.SetLabels(util.MergeMapOverrideWithDst(auxO.GetLabels(), map[string]string{oam.LabelAddonAuxiliaryName: k}))
			res = append(res, auxO)
		}
	}
	return &app, res, nil
}

// generateAppFramework generate application from yaml defined by template.yaml or cue file from template.cue
func generateAppFramework(addon *InstallPackage, parameters map[string]interface{}) (*v1beta1.Application, []*unstructured.Unstructured, error) {
	if len(addon.AppCueTemplate.Data) != 0 && addon.AppTemplate != nil {
		return nil, nil, ErrBothCueAndYamlTmpl
	}

	var app *v1beta1.Application
	var auxiliaryObjects []*unstructured.Unstructured
	var err error
	if len(addon.AppCueTemplate.Data) != 0 {
		app, auxiliaryObjects, err = renderAppAccordingToCueTemplate(addon, parameters)
		if err != nil {
			return nil, nil, err
		}
	} else {
		app = addon.AppTemplate
		if app == nil {
			app = &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ApplicationKind},
			}
		}
		if app.Spec.Components == nil {
			app.Spec.Components = []common2.ApplicationComponent{}
		}
	}

	app.Name = addonutil.Addon2AppName(addon.Name)
	// force override the namespace defined vela with DefaultVelaNS,this value can be modified by Env
	app.SetNamespace(types.DefaultKubeVelaNS)
	if app.Labels == nil {
		app.Labels = make(map[string]string)
	}
	app.Labels[oam.LabelAddonName] = addon.Name
	app.Labels[oam.LabelAddonVersion] = addon.Version

	for _, aux := range auxiliaryObjects {
		aux.SetLabels(util.MergeMapOverrideWithDst(aux.GetLabels(), map[string]string{oam.LabelAddonName: addon.Name, oam.LabelAddonVersion: addon.Version}))
	}

	return app, auxiliaryObjects, nil
}

func renderAppAccordingToCueTemplate(addon *InstallPackage, args map[string]interface{}) (*v1beta1.Application, []*unstructured.Unstructured, error) {
	r := addonCueTemplateRender{
		addon:     addon,
		inputArgs: args,
	}
	return r.renderApp()
}

// renderCompAccordingCUETemplate will return a component from cue template
func renderCompAccordingCUETemplate(cueTemplate ElementFile, addon *InstallPackage, args map[string]interface{}) (*common2.ApplicationComponent, error) {
	comp := common2.ApplicationComponent{}

	r := addonCueTemplateRender{
		addon:     addon,
		inputArgs: args,
	}
	if err := r.toObject(cueTemplate.Data, renderOutputCuePath, &comp); err != nil {
		return nil, fmt.Errorf("error rendering file %s: %w", cueTemplate.Name, err)
	}
	// If the name of component has been set, just keep it, otherwise will set with file name.
	if len(comp.Name) == 0 {
		fileName := strings.ReplaceAll(cueTemplate.Name, path.Ext(cueTemplate.Name), "")
		comp.Name = strings.ReplaceAll(fileName, ".", "-")
	}
	return &comp, nil
}

// RenderApp render a K8s application
func RenderApp(ctx context.Context, addon *InstallPackage, k8sClient client.Client, args map[string]interface{}) (*v1beta1.Application, []*unstructured.Unstructured, error) {
	if args == nil {
		args = map[string]interface{}{}
	}
	app, auxiliaryObjects, err := generateAppFramework(addon, args)
	if err != nil {
		return nil, nil, err
	}
	app.Spec.Components = append(app.Spec.Components, renderNeededNamespaceAsComps(addon)...)

	resources, err := renderResources(addon, args)
	if err != nil {
		return nil, nil, err
	}
	app.Spec.Components = append(app.Spec.Components, resources...)

	// for legacy addons those hasn't define policy in template.cue but still want to deploy runtime cluster
	// attach topology policy to application.
	if checkNeedAttachTopologyPolicy(app, addon) {
		if err := attachPolicyForLegacyAddon(ctx, app, addon, args, k8sClient); err != nil {
			return nil, nil, err
		}
	}
	return app, auxiliaryObjects, nil
}

func attachPolicyForLegacyAddon(ctx context.Context, app *v1beta1.Application, addon *InstallPackage, args map[string]interface{}, k8sClient client.Client) error {
	deployClusters, err := checkDeployClusters(ctx, k8sClient, args)
	if err != nil {
		return err
	}

	if !isDeployToRuntime(addon) {
		return nil
	}

	if len(deployClusters) == 0 {
		// empty cluster args deploy to all clusters
		clusterSelector := map[string]interface{}{
			// empty labelSelector means deploy resources to all clusters
			ClusterLabelSelector: map[string]string{},
		}
		properties, err := json.Marshal(clusterSelector)
		if err != nil {
			return err
		}
		policy := v1beta1.AppPolicy{
			Name:       addonAllClusterPolicy,
			Type:       v1alpha1.TopologyPolicyType,
			Properties: &runtime.RawExtension{Raw: properties},
		}
		app.Spec.Policies = append(app.Spec.Policies, policy)
	} else {
		var found bool
		for _, c := range deployClusters {
			if c == multicluster.ClusterLocalName {
				found = true
				break
			}
		}
		if !found {
			deployClusters = append(deployClusters, multicluster.ClusterLocalName)
		}
		// deploy to specified clusters
		if app.Spec.Policies == nil {
			app.Spec.Policies = []v1beta1.AppPolicy{}
		}
		body, err := json.Marshal(map[string][]string{types.ClustersArg: deployClusters})
		if err != nil {
			return err
		}
		app.Spec.Policies = append(app.Spec.Policies, v1beta1.AppPolicy{
			Name:       specifyAddonClustersTopologyPolicy,
			Type:       v1alpha1.TopologyPolicyType,
			Properties: &runtime.RawExtension{Raw: body},
		})
	}

	return nil
}

func renderResources(addon *InstallPackage, args map[string]interface{}) ([]common2.ApplicationComponent, error) {
	var resources []common2.ApplicationComponent
	if len(addon.YAMLTemplates) != 0 {
		comp, err := renderK8sObjectsComponent(addon.YAMLTemplates, addon.Name)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *comp)
	}

	for _, tmpl := range addon.CUETemplates {
		comp, err := renderCompAccordingCUETemplate(tmpl, addon, args)
		if err != nil && strings.Contains(err.Error(), "var(path=output) not exist") {
			continue
		}
		if err != nil {
			return nil, NewAddonError(fmt.Sprintf("fail to render cue template %s", err.Error()))
		}
		resources = append(resources, *comp)
	}
	return resources, nil
}

// checkNeedAttachTopologyPolicy will check this addon want to deploy to runtime-cluster, but application template doesn't specify the
// topology policy, then will attach the policy to application automatically.
func checkNeedAttachTopologyPolicy(app *v1beta1.Application, addon *InstallPackage) bool {
	if !isDeployToRuntime(addon) {
		return false
	}
	for _, policy := range app.Spec.Policies {
		if policy.Type == v1alpha1.TopologyPolicyType {
			return false
		}
	}
	return true
}

func isDeployToRuntime(addon *InstallPackage) bool {
	if addon.DeployTo == nil {
		return false
	}
	return addon.DeployTo.RuntimeCluster || addon.DeployTo.LegacyRuntimeCluster
}
