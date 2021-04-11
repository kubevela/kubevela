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

package appfile

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	json2cue "cuelang.org/go/encoding/json"
	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// constant error information
const (
	errInvalidValueType = "require %q type parameter value"
)

// Workload is component
type Workload struct {
	Name               string
	Type               string
	CapabilityCategory types.CapabilityCategory
	Params             map[string]interface{}
	Traits             []*Trait
	Scopes             []Scope
	FullTemplate       *Template
	engine             definition.AbstractEngine
	// OutputSecretName is the secret name which this workload will generate after it successfully generate a cloud resource
	OutputSecretName string
	// RequiredSecrets stores secret names which the workload needs from cloud resource component and its context
	RequiredSecrets []process.RequiredSecrets
	UserConfigs     []map[string]string
}

// GetUserConfigName get user config from AppFile, it will contain config file in it.
func (wl *Workload) GetUserConfigName() string {
	if wl.Params == nil {
		return ""
	}
	t, ok := wl.Params[AppfileBuiltinConfig]
	if !ok {
		return ""
	}
	ts, ok := t.(string)
	if !ok {
		return ""
	}
	return ts
}

// EvalContext eval workload template and set result to context
func (wl *Workload) EvalContext(ctx process.Context) error {
	return wl.engine.Complete(ctx, wl.FullTemplate.TemplateStr, wl.Params)
}

// EvalStatus eval workload status
func (wl *Workload) EvalStatus(ctx process.Context, cli client.Client, ns string) (string, error) {
	return wl.engine.Status(ctx, cli, ns, wl.FullTemplate.CustomStatus)
}

// EvalHealth eval workload health check
func (wl *Workload) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	return wl.engine.HealthCheck(ctx, client, namespace, wl.FullTemplate.Health)
}

// IsCloudResourceProducer checks whether a workload is cloud resource producer role
func (wl *Workload) IsCloudResourceProducer() bool {
	var existed bool
	_, existed = wl.Params[process.OutputSecretName]
	return existed
}

// IsCloudResourceConsumer checks whether a workload is cloud resource consumer role
func (wl *Workload) IsCloudResourceConsumer() bool {
	requiredSecretTag := strings.TrimRight(InsertSecretToTag, "=")
	matched, err := regexp.Match(regexp.QuoteMeta(requiredSecretTag), []byte(wl.FullTemplate.TemplateStr))
	if err != nil || !matched {
		return false
	}
	return true
}

// Scope defines the scope of workload
type Scope struct {
	Name string
	GVK  schema.GroupVersionKind
}

// Trait is ComponentTrait
type Trait struct {
	// The Name is name of TraitDefinition, actually it's a type of the trait instance
	Name               string
	CapabilityCategory types.CapabilityCategory
	Params             map[string]interface{}

	Template           string
	HealthCheckPolicy  string
	CustomStatusFormat string

	FullTemplate *Template
	engine       definition.AbstractEngine
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return trait.engine.Complete(ctx, trait.Template, trait.Params)
}

// EvalStatus eval trait status
func (trait *Trait) EvalStatus(ctx process.Context, cli client.Client, ns string) (string, error) {
	return trait.engine.Status(ctx, cli, ns, trait.CustomStatusFormat)
}

// EvalHealth eval trait health check
func (trait *Trait) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	return trait.engine.HealthCheck(ctx, client, namespace, trait.HealthCheckPolicy)
}

// Appfile describes application
type Appfile struct {
	Name         string
	Namespace    string
	RevisionName string
	Workloads    []*Workload
}

// TemplateValidate validate Template format
func (af *Appfile) TemplateValidate() error {
	return nil
}

// GenerateApplicationConfiguration converts an appFile to applicationConfig & Components
func (af *Appfile) GenerateApplicationConfiguration() (*v1alpha2.ApplicationConfiguration,
	[]*v1alpha2.Component, error) {
	appconfig := &v1alpha2.ApplicationConfiguration{}
	appconfig.SetGroupVersionKind(v1alpha2.ApplicationConfigurationGroupVersionKind)
	appconfig.Name = af.Name
	appconfig.Namespace = af.Namespace

	if appconfig.Labels == nil {
		appconfig.Labels = map[string]string{}
	}
	appconfig.Labels[oam.LabelAppName] = af.Name

	var components []*v1alpha2.Component

	for _, wl := range af.Workloads {
		var (
			comp   *v1alpha2.Component
			acComp *v1alpha2.ApplicationConfigurationComponent
			err    error
		)
		switch wl.CapabilityCategory {
		case types.HelmCategory:
			comp, acComp, err = generateComponentFromHelmModule(wl, af.Name, af.RevisionName, af.Namespace)
			if err != nil {
				return nil, nil, err
			}
		case types.KubeCategory:
			comp, acComp, err = generateComponentFromKubeModule(wl, af.Name, af.RevisionName, af.Namespace)
			if err != nil {
				return nil, nil, err
			}
		default:
			comp, acComp, err = generateComponentFromCUEModule(wl, af.Name, af.RevisionName, af.Namespace)
			if err != nil {
				return nil, nil, err
			}
		}
		components = append(components, comp)
		appconfig.Spec.Components = append(appconfig.Spec.Components, *acComp)
	}
	return appconfig, components, nil
}

// PrepareProcessContext prepares a DSL process Context
func PrepareProcessContext(wl *Workload, applicationName, revision, namespace string) (process.Context, error) {
	pCtx := process.NewContext(namespace, wl.Name, applicationName, revision)
	pCtx.InsertSecrets(wl.OutputSecretName, wl.RequiredSecrets)
	if len(wl.UserConfigs) > 0 {
		pCtx.SetConfigs(wl.UserConfigs)
	}
	if err := wl.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", applicationName, namespace)
	}
	return pCtx, nil
}

func generateComponentFromCUEModule(wl *Workload, appName, revision, ns string) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	var (
		outputSecretName string
		err              error
	)
	if wl.IsCloudResourceProducer() {
		outputSecretName, err = GetOutputSecretNames(wl)
		if err != nil {
			return nil, nil, err
		}
		wl.OutputSecretName = outputSecretName
	}
	pCtx, err := PrepareProcessContext(wl, appName, revision, ns)
	if err != nil {
		return nil, nil, err
	}
	for _, tr := range wl.Traits {
		if err := tr.EvalContext(pCtx); err != nil {
			return nil, nil, errors.Wrapf(err, "evaluate template trait=%s app=%s", tr.Name, wl.Name)
		}
	}
	var comp *v1alpha2.Component
	var acComp *v1alpha2.ApplicationConfigurationComponent
	comp, acComp, err = evalWorkloadWithContext(pCtx, wl, appName, wl.Name)
	if err != nil {
		return nil, nil, err
	}
	comp.Name = wl.Name
	acComp.ComponentName = comp.Name

	for _, sc := range wl.Scopes {
		acComp.Scopes = append(acComp.Scopes, v1alpha2.ComponentScope{ScopeReference: v1alpha1.TypedReference{
			APIVersion: sc.GVK.GroupVersion().String(),
			Kind:       sc.GVK.Kind,
			Name:       sc.Name,
		}})
	}
	if len(comp.Namespace) == 0 {
		comp.Namespace = ns
	}
	if comp.Labels == nil {
		comp.Labels = map[string]string{}
	}
	comp.Labels[oam.LabelAppName] = appName
	comp.SetGroupVersionKind(v1alpha2.ComponentGroupVersionKind)

	return comp, acComp, nil
}

// evalWorkloadWithContext evaluate the workload's template to generate component and ACComponent
func evalWorkloadWithContext(pCtx process.Context, wl *Workload, appName, compName string) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	base, assists := pCtx.Output()
	componentWorkload, err := base.Unstructured()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "evaluate base template component=%s app=%s", compName, appName)
	}

	var commonLabels = definition.GetCommonLabels(pCtx.BaseContextLabels())
	util.AddLabels(componentWorkload, util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.WorkloadTypeLabel: wl.Type}))

	component := &v1alpha2.Component{}
	// we need to marshal the workload to byte array before sending them to the k8s
	component.Spec.Workload = util.Object2RawExtension(componentWorkload)

	acComponent := &v1alpha2.ApplicationConfigurationComponent{}
	for _, assist := range assists {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "evaluate trait=%s template for component=%s app=%s", assist.Name, compName, appName)
		}
		labels := util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.TraitTypeLabel: assist.Type})
		if assist.Name != "" {
			labels[oam.TraitResource] = assist.Name
		}
		util.AddLabels(tr, labels)
		acComponent.Traits = append(acComponent.Traits, v1alpha2.ComponentTrait{
			// we need to marshal the trait to byte array before sending them to the k8s
			Trait: util.Object2RawExtension(tr),
		})
	}
	return component, acComponent, nil
}

func generateComponentFromKubeModule(wl *Workload, appName, revision, ns string) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	kubeObj := &unstructured.Unstructured{}
	err := json.Unmarshal(wl.FullTemplate.Kube.Template.Raw, kubeObj)
	if err != nil {
		return nil, nil, errors.Wrap(err, "cannot decode Kube template into K8s object")
	}

	paramValues, err := resolveKubeParameters(wl.FullTemplate.Kube.Parameters, wl.Params)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "cannot resolve parameter settings")
	}
	if err := setParameterValuesToKubeObj(kubeObj, paramValues); err != nil {
		return nil, nil, errors.WithMessage(err, "cannot set parameters value")
	}

	// convert structured kube obj into CUE (go ==marshal==> json ==decoder==> cue)
	objRaw, err := kubeObj.MarshalJSON()
	if err != nil {
		return nil, nil, errors.Wrap(err, "cannot marshal kube object")
	}
	ins, err := json2cue.Decode(&cue.Runtime{}, "", objRaw)
	if err != nil {
		return nil, nil, errors.Wrap(err, "cannot decode object into CUE")
	}
	cueRaw, err := format.Node(ins.Value().Syntax())
	if err != nil {
		return nil, nil, errors.Wrap(err, "cannot format CUE")
	}

	// NOTE a hack way to enable using CUE capabilities on KUBE schematic workload
	wl.FullTemplate.TemplateStr = fmt.Sprintf(`
output: { 
	%s 
}`, string(cueRaw))

	// re-use the way CUE module generates comp & acComp
	comp, acComp, err := generateComponentFromCUEModule(wl, appName, revision, ns)
	if err != nil {
		return nil, nil, err
	}
	return comp, acComp, nil
}

// a helper map whose key is parameter name
type paramValueSettings map[string]paramValueSetting
type paramValueSetting struct {
	Value      interface{}
	ValueType  common.ParameterValueType
	FieldPaths []string
}

func resolveKubeParameters(params []common.KubeParameter, settings map[string]interface{}) (paramValueSettings, error) {
	supported := map[string]*common.KubeParameter{}
	for _, p := range params {
		supported[p.Name] = p.DeepCopy()
	}

	values := make(paramValueSettings)
	for name, v := range settings {
		// check unsupported parameter setting
		if supported[name] == nil {
			return nil, errors.Errorf("unsupported parameter %q", name)
		}
		// construct helper map
		values[name] = paramValueSetting{
			Value:      v,
			ValueType:  supported[name].ValueType,
			FieldPaths: supported[name].FieldPaths,
		}
	}

	// check required parameter
	for _, p := range params {
		if p.Required != nil && *p.Required {
			if _, ok := values[p.Name]; !ok {
				return nil, errors.Errorf("require parameter %q", p.Name)
			}
		}
	}
	return values, nil
}

func setParameterValuesToKubeObj(obj *unstructured.Unstructured, values paramValueSettings) error {
	paved := fieldpath.Pave(obj.Object)
	for paramName, v := range values {
		for _, f := range v.FieldPaths {
			switch v.ValueType {
			case common.StringType:
				vString, ok := v.Value.(string)
				if !ok {
					return errors.Errorf(errInvalidValueType, v.ValueType)
				}
				if err := paved.SetString(f, vString); err != nil {
					return errors.Wrapf(err, "cannot set parameter %q to field %q", paramName, f)
				}
			case common.NumberType:
				switch v.Value.(type) {
				case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
					if err := paved.SetValue(f, v.Value); err != nil {
						return errors.Wrapf(err, "cannot set parameter %q to field %q", paramName, f)
					}
				default:
					return errors.Errorf(errInvalidValueType, v.ValueType)
				}
			case common.BooleanType:
				vBoolean, ok := v.Value.(bool)
				if !ok {
					return errors.Errorf(errInvalidValueType, v.ValueType)
				}
				if err := paved.SetValue(f, vBoolean); err != nil {
					return errors.Wrapf(err, "cannot set parameter %q to field %q", paramName, f)
				}
			}
		}
	}
	return nil
}

func generateComponentFromHelmModule(wl *Workload, appName, revision, ns string) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	gv, err := schema.ParseGroupVersion(wl.FullTemplate.Reference.APIVersion)
	if err != nil {
		return nil, nil, err
	}
	targetWorkloadGVK := gv.WithKind(wl.FullTemplate.Reference.Kind)

	// NOTE this is a hack way to enable using CUE module capabilities on Helm module workload
	// construct an empty base workload according to its GVK
	wl.FullTemplate.TemplateStr = fmt.Sprintf(`
output: {
	apiVersion: "%s"
	kind: "%s"
}`, targetWorkloadGVK.GroupVersion().String(), targetWorkloadGVK.Kind)

	// re-use the way CUE module generates comp & acComp
	comp, acComp, err := generateComponentFromCUEModule(wl, appName, revision, ns)
	if err != nil {
		return nil, nil, err
	}

	release, repo, err := helm.RenderHelmReleaseAndHelmRepo(wl.FullTemplate.Helm, wl.Name, appName, ns, wl.Params)
	if err != nil {
		return nil, nil, err
	}
	rlsBytes, err := json.Marshal(release.Object)
	if err != nil {
		return nil, nil, err
	}
	repoBytes, err := json.Marshal(repo.Object)
	if err != nil {
		return nil, nil, err
	}
	comp.Spec.Helm = &common.Helm{
		Release:    runtime.RawExtension{Raw: rlsBytes},
		Repository: runtime.RawExtension{Raw: repoBytes},
	}
	return comp, acComp, nil
}
