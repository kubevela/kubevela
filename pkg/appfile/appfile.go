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
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"

	"github.com/oam-dev/kubevela/pkg/appfile/config"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/format"
	json2cue "cuelang.org/go/encoding/json"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/helm"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// constant error information
const (
	errInvalidValueType                                = "require %q type parameter value"
	errTerraformConfigurationIsNotSet                  = "terraform configuration is not set"
	errFailToConvertTerraformComponentProperties       = "failed to convert Terraform component properties"
	errTerraformNameOfWriteConnectionSecretToRefNotSet = "the name of writeConnectionSecretToRef of terraform component is not set"
)

// WriteConnectionSecretToRefKey is used to create a secret for cloud resource connection
const WriteConnectionSecretToRefKey = "writeConnectionSecretToRef"

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
	// ConfigNotReady indicates there's RequiredSecrets and UserConfigs but they're not ready yet.
	ConfigNotReady bool
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
	return wl.engine.Status(ctx, cli, ns, wl.FullTemplate.CustomStatus, wl.Params)
}

// EvalHealth eval workload health check
func (wl *Workload) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	return wl.engine.HealthCheck(ctx, client, namespace, wl.FullTemplate.Health)
}

// IsSecretProducer checks whether a workload is cloud resource producer role
func (wl *Workload) IsSecretProducer() bool {
	var existed bool
	_, existed = wl.Params[process.OutputSecretName]
	return existed
}

// IsSecretConsumer checks whether a workload is cloud resource consumer role
func (wl *Workload) IsSecretConsumer() bool {
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

	// RequiredSecrets stores secret names which the trait needs from cloud resource component and its context
	RequiredSecrets []process.RequiredSecrets

	FullTemplate *Template
	engine       definition.AbstractEngine
}

// EvalContext eval trait template and set result to context
func (trait *Trait) EvalContext(ctx process.Context) error {
	return trait.engine.Complete(ctx, trait.Template, trait.Params)
}

// EvalStatus eval trait status
func (trait *Trait) EvalStatus(ctx process.Context, cli client.Client, ns string) (string, error) {
	return trait.engine.Status(ctx, cli, ns, trait.CustomStatusFormat, trait.Params)
}

// EvalHealth eval trait health check
func (trait *Trait) EvalHealth(ctx process.Context, client client.Client, namespace string) (bool, error) {
	return trait.engine.HealthCheck(ctx, client, namespace, trait.HealthCheckPolicy)
}

// IsSecretConsumer checks whether a trait is cloud resource consumer role
func (trait *Trait) IsSecretConsumer() bool {
	requiredSecretTag := strings.TrimRight(InsertSecretToTag, "=")
	matched, err := regexp.Match(regexp.QuoteMeta(requiredSecretTag), []byte(trait.FullTemplate.TemplateStr))
	if err != nil || !matched {
		return false
	}
	return true
}

// Appfile describes application
type Appfile struct {
	Name         string
	Namespace    string
	RevisionName string
	Workloads    []*Workload

	Policies      []*Workload
	WorkflowSteps []v1beta1.WorkflowStep
}

// GenerateWorkflowAndPolicy generates workflow steps and policies from an appFile
func (af *Appfile) GenerateWorkflowAndPolicy(m discoverymapper.DiscoveryMapper, cli client.Client, pd *packages.PackageDiscover) (policies []*unstructured.Unstructured, steps []wfTypes.TaskRunner, err error) {
	policies, err = af.generateUnstructureds(af.Policies)
	if err != nil {
		return
	}

	steps, err = af.generateSteps(m, cli, pd)
	return
}

func (af *Appfile) generateUnstructureds(workloads []*Workload) ([]*unstructured.Unstructured, error) {
	uns := []*unstructured.Unstructured{}
	for _, wl := range workloads {
		un, err := generateUnstructuredFromCUEModule(wl, af.Name, af.RevisionName, af.Namespace)
		if err != nil {
			return nil, err
		}
		uns = append(uns, un)
	}
	return uns, nil
}

func (af *Appfile) generateSteps(dm discoverymapper.DiscoveryMapper, cli client.Client, pd *packages.PackageDiscover) ([]wfTypes.TaskRunner, error) {
	loadTaskTemplate := func(ctx context.Context, name string) (string, error) {
		templ, err := LoadTemplate(context.Background(), dm, cli, name, types.TypeWorkflowStep)
		if err != nil {
			return "", err
		}
		schematic := templ.WorkflowStepDefinition.Spec.Schematic
		if schematic != nil && schematic.CUE != nil {
			return schematic.CUE.Template, nil
		}
		return "", errors.New("custom workflowStep only support cue")
	}

	taskDiscover := tasks.NewTaskDiscover(cli, pd, loadTaskTemplate)
	var tasks []wfTypes.TaskRunner
	for _, step := range af.WorkflowSteps {
		genTask, err := taskDiscover.GetTaskGenerator(step.Type)
		if err!=nil{
			return nil, err
		}
		task, err := genTask(step)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func generateUnstructuredFromCUEModule(wl *Workload, appName, revision, ns string) (*unstructured.Unstructured, error) {
	pCtx, err := PrepareProcessContext(wl, appName, revision, ns)
	if err != nil {
		return nil, err
	}
	return makeWorkloadWithContext(pCtx, wl, ns, appName)
}

// GenerateComponentManifests converts an appFile to a slice of ComponentManifest
func (af *Appfile) GenerateComponentManifests() ([]*types.ComponentManifest, error) {
	compManifests := make([]*types.ComponentManifest, len(af.Workloads))
	for i, wl := range af.Workloads {
		cm, err := af.GenerateComponentManifest(wl)
		if err != nil {
			return nil, err
		}
		compManifests[i] = cm
	}
	return compManifests, nil
}

// GenerateComponentManifest generate only one ComponentManifest
func (af *Appfile) GenerateComponentManifest(wl *Workload) (*types.ComponentManifest, error) {
	if wl.ConfigNotReady {
		return &types.ComponentManifest{
			Name:                 wl.Name,
			InsertConfigNotReady: true,
		}, nil
	}
	switch wl.CapabilityCategory {
	case types.HelmCategory:
		return generateComponentFromHelmModule(wl, af.Name, af.RevisionName, af.Namespace)
	case types.KubeCategory:
		return generateComponentFromKubeModule(wl, af.Name, af.RevisionName, af.Namespace)
	case types.TerraformCategory:
		return generateComponentFromTerraformModule(wl, af.Name, af.RevisionName, af.Namespace)
	default:
		return generateComponentFromCUEModule(wl, af.Name, af.RevisionName, af.Namespace)
	}
}

// PrepareProcessContext prepares a DSL process Context
func PrepareProcessContext(wl *Workload, applicationName, revision, namespace string) (process.Context, error) {
	pCtx := NewBasicContext(wl, applicationName, revision, namespace)
	if err := wl.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", applicationName, namespace)
	}
	return pCtx, nil
}

// NewBasicContext prepares a basic DSL process Context
func NewBasicContext(wl *Workload, applicationName, revision, namespace string) process.Context {
	pCtx := process.NewContext(namespace, wl.Name, applicationName, revision)
	pCtx.InsertSecrets(wl.OutputSecretName, wl.RequiredSecrets)
	if len(wl.UserConfigs) > 0 {
		pCtx.SetConfigs(wl.UserConfigs)
	}
	return pCtx
}

// GetSecretAndConfigs will get secrets and configs the workload requires
func GetSecretAndConfigs(cli client.Client, workload *Workload, appName, ns string) error {
	if workload.IsSecretConsumer() {
		requiredSecrets, err := parseInsertSecretTo(context.TODO(), cli, ns, workload.FullTemplate.TemplateStr, workload.Params)
		if err != nil {
			return err
		}
		workload.RequiredSecrets = requiredSecrets
	}

	for _, tr := range workload.Traits {
		if tr.IsSecretConsumer() {
			requiredSecrets, err := parseInsertSecretTo(context.TODO(), cli, ns, tr.FullTemplate.TemplateStr, tr.Params)
			if err != nil {
				return err
			}
			tr.RequiredSecrets = requiredSecrets
		}
	}

	userConfig := workload.GetUserConfigName()
	if userConfig != "" {
		cg := config.Configmap{Client: cli}
		// TODO(wonderflow): envName should not be namespace when we have serverside env
		var envName = ns
		data, err := cg.GetConfigData(config.GenConfigMapName(appName, workload.Name, userConfig), envName)
		if err != nil {
			return errors.Wrapf(err, "get config=%s for app=%s in namespace=%s", userConfig, appName, ns)
		}
		workload.UserConfigs = data
	}
	return nil
}

func generateComponentFromCUEModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	pCtx, err := PrepareProcessContext(wl, appName, revision, ns)
	if err != nil {
		return nil, err
	}
	return baseGenerateComponent(pCtx, wl, appName, ns)
}

func generateComponentFromTerraformModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	pCtx := NewBasicContext(wl, appName, revision, ns)
	return baseGenerateComponent(pCtx, wl, appName, ns)
}

func baseGenerateComponent(pCtx process.Context, wl *Workload, appName, ns string) (*types.ComponentManifest, error) {
	var (
		outputSecretName string
		err              error
	)
	if wl.IsSecretProducer() {
		outputSecretName, err = GetOutputSecretNames(wl)
		if err != nil {
			return nil, err
		}
		wl.OutputSecretName = outputSecretName
	}
	for _, tr := range wl.Traits {
		pCtx.InsertSecrets("", tr.RequiredSecrets)
		if err := tr.EvalContext(pCtx); err != nil {
			return nil, errors.Wrapf(err, "evaluate template trait=%s app=%s", tr.Name, wl.Name)
		}
	}
	compManifest, err := evalWorkloadWithContext(pCtx, wl, ns, appName, wl.Name)
	if err != nil {
		return nil, err
	}
	compManifest.Name = wl.Name

	compManifest.Scopes = make([]*corev1.ObjectReference, len(wl.Scopes))
	for i, s := range wl.Scopes {
		compManifest.Scopes[i] = &corev1.ObjectReference{
			APIVersion: s.GVK.GroupVersion().String(),
			Kind:       s.GVK.Kind,
			Name:       s.Name,
		}
	}
	return compManifest, nil
}

// makeWorkloadWithContext evaluate the workload's template to unstructured resource.
func makeWorkloadWithContext(pCtx process.Context, wl *Workload, ns, appName string) (*unstructured.Unstructured, error) {
	var (
		workload *unstructured.Unstructured
		err      error
	)
	base, _ := pCtx.Output()
	switch wl.CapabilityCategory {
	case types.TerraformCategory:
		workload, err = generateTerraformConfigurationWorkload(wl, ns)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to generate Terraform Configuration workload for workload %s", wl.Name)
		}
	default:
		workload, err = base.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate base template component=%s app=%s", wl.Name, appName)
		}
	}
	commonLabels := definition.GetCommonLabels(pCtx.BaseContextLabels())
	util.AddLabels(workload, util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.WorkloadTypeLabel: wl.Type}))
	return workload, nil
}

// evalWorkloadWithContext evaluate the workload's template to generate component manifest
func evalWorkloadWithContext(pCtx process.Context, wl *Workload, ns, appName, compName string) (*types.ComponentManifest, error) {
	compManifest := &types.ComponentManifest{}
	workload, err := makeWorkloadWithContext(pCtx, wl, ns, appName)
	if err != nil {
		return nil, err
	}
	compManifest.StandardWorkload = workload

	_, assists := pCtx.Output()
	compManifest.Traits = make([]*unstructured.Unstructured, len(assists))
	commonLabels := definition.GetCommonLabels(pCtx.BaseContextLabels())
	for i, assist := range assists {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, errors.Wrapf(err, "evaluate trait=%s template for component=%s app=%s", assist.Name, compName, appName)
		}
		labels := util.MergeMapOverrideWithDst(commonLabels, map[string]string{oam.TraitTypeLabel: assist.Type})
		if assist.Name != "" {
			labels[oam.TraitResource] = assist.Name
		}
		util.AddLabels(tr, labels)
		compManifest.Traits[i] = tr
	}
	return compManifest, nil
}

// GenerateCUETemplate generate CUE Template from Kube module and Helm module
func GenerateCUETemplate(wl *Workload) (string, error) {
	var templateStr string
	switch wl.CapabilityCategory {
	case types.KubeCategory:
		kubeObj := &unstructured.Unstructured{}

		err := json.Unmarshal(wl.FullTemplate.Kube.Template.Raw, kubeObj)
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot decode Kube template into K8s object")
		}

		paramValues, err := resolveKubeParameters(wl.FullTemplate.Kube.Parameters, wl.Params)
		if err != nil {
			return templateStr, errors.WithMessage(err, "cannot resolve parameter settings")
		}
		if err := setParameterValuesToKubeObj(kubeObj, paramValues); err != nil {
			return templateStr, errors.WithMessage(err, "cannot set parameters value")
		}

		// convert structured kube obj into CUE (go ==marshal==> json ==decoder==> cue)
		objRaw, err := kubeObj.MarshalJSON()
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot marshal kube object")
		}
		ins, err := json2cue.Decode(&cue.Runtime{}, "", objRaw)
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot decode object into CUE")
		}
		cueRaw, err := format.Node(ins.Value().Syntax())
		if err != nil {
			return templateStr, errors.Wrap(err, "cannot format CUE")
		}

		// NOTE a hack way to enable using CUE capabilities on KUBE schematic workload
		templateStr = fmt.Sprintf(`
output: { 
%s 
}`, string(cueRaw))
	case types.HelmCategory:
		gv, err := schema.ParseGroupVersion(wl.FullTemplate.Reference.Definition.APIVersion)
		if err != nil {
			return templateStr, err
		}
		targetWorkloadGVK := gv.WithKind(wl.FullTemplate.Reference.Definition.Kind)
		// NOTE this is a hack way to enable using CUE module capabilities on Helm module workload
		// construct an empty base workload according to its GVK
		templateStr = fmt.Sprintf(`
output: {
	apiVersion: "%s"
	kind: "%s"
}`, targetWorkloadGVK.GroupVersion().String(), targetWorkloadGVK.Kind)
	default:
	}
	return templateStr, nil
}

func generateComponentFromKubeModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	templateStr, err := GenerateCUETemplate(wl)
	if err != nil {
		return nil, err
	}
	wl.FullTemplate.TemplateStr = templateStr

	// re-use the way CUE module generates comp & acComp
	compManifest, err := generateComponentFromCUEModule(wl, appName, revision, ns)
	if err != nil {
		return nil, err
	}
	return compManifest, nil
}

func generateTerraformConfigurationWorkload(wl *Workload, ns string) (*unstructured.Unstructured, error) {
	if wl.FullTemplate.Terraform.Configuration == "" {
		return nil, errors.New(errTerraformConfigurationIsNotSet)
	}
	params, err := json.Marshal(wl.Params)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	configuration := terraformapi.Configuration{
		TypeMeta:   metav1.TypeMeta{APIVersion: "terraform.core.oam.dev/v1beta1", Kind: "Configuration"},
		ObjectMeta: metav1.ObjectMeta{Name: wl.Name, Namespace: ns},
	}

	switch wl.FullTemplate.Terraform.Type {
	case "hcl":
		configuration.Spec.HCL = wl.FullTemplate.Terraform.Configuration
	case "json":
		configuration.Spec.JSON = wl.FullTemplate.Terraform.Configuration
	}

	// 1. parse writeConnectionSecretToRef
	if err := json.Unmarshal(params, &configuration.Spec); err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	if configuration.Spec.WriteConnectionSecretToReference != nil {
		if configuration.Spec.WriteConnectionSecretToReference.Name == "" {
			return nil, errors.New(errTerraformNameOfWriteConnectionSecretToRefNotSet)
		}
		// set namespace for writeConnectionSecretToRef, developer needn't manually set it
		if configuration.Spec.WriteConnectionSecretToReference.Namespace == "" {
			configuration.Spec.WriteConnectionSecretToReference.Namespace = ns
		}
	}

	// 2. parse variable
	variableRaw := &runtime.RawExtension{}
	if err := json.Unmarshal(params, &variableRaw); err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}

	variableMap, err := util.RawExtension2Map(variableRaw)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}
	delete(variableMap, WriteConnectionSecretToRefKey)

	data, err := json.Marshal(variableMap)
	if err != nil {
		return nil, errors.Wrap(err, errFailToConvertTerraformComponentProperties)
	}
	configuration.Spec.Variable = &runtime.RawExtension{Raw: data}
	raw := util.Object2RawExtension(&configuration)
	return util.RawExtension2Unstructured(&raw)
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

func generateComponentFromHelmModule(wl *Workload, appName, revision, ns string) (*types.ComponentManifest, error) {
	templateStr, err := GenerateCUETemplate(wl)
	if err != nil {
		return nil, err
	}
	wl.FullTemplate.TemplateStr = templateStr

	// re-use the way CUE module generates comp & acComp
	compManifest := &types.ComponentManifest{
		Name: wl.Name,
	}
	if wl.FullTemplate.Reference.Type != types.AutoDetectWorkloadDefinition {
		compManifest, err = generateComponentFromCUEModule(wl, appName, revision, ns)
		if err != nil {
			return nil, err
		}
	}

	rls, repo, err := helm.RenderHelmReleaseAndHelmRepo(wl.FullTemplate.Helm, wl.Name, appName, ns, wl.Params)
	if err != nil {
		return nil, err
	}
	compManifest.PackagedWorkloadResources = []*unstructured.Unstructured{rls, repo}
	return compManifest, nil
}
