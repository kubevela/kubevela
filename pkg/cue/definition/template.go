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

package definition

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/cue/model"
	"github.com/kubevela/workflow/pkg/cue/model/sets"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/packages"
	"github.com/kubevela/workflow/pkg/cue/process"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/cue/task"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// OutputFieldName is the name of the struct contains the CR data
	OutputFieldName = velaprocess.OutputFieldName
	// OutputsFieldName is the name of the struct contains the map[string]CR data
	OutputsFieldName = velaprocess.OutputsFieldName
	// PatchFieldName is the name of the struct contains the patch of CR data
	PatchFieldName = "patch"
	// PatchOutputsFieldName is the name of the struct contains the patch of outputs CR data
	PatchOutputsFieldName = "patchOutputs"
	// CustomMessage defines the custom message in definition template
	CustomMessage = "message"
	// HealthCheckPolicy defines the health check policy in definition template
	HealthCheckPolicy = "isHealth"
	// ErrsFieldName check if errors contained in the cue
	ErrsFieldName = "errs"
)

const (
	// AuxiliaryWorkload defines the extra workload obj from a workloadDefinition,
	// e.g. a workload composed by deployment and service, the service will be marked as AuxiliaryWorkload
	AuxiliaryWorkload = "AuxiliaryWorkload"
)

// AbstractEngine defines Definition's Render interface
type AbstractEngine interface {
	Complete(ctx process.Context, abstractTemplate string, params interface{}) error
	HealthCheck(templateContext map[string]interface{}, healthPolicyTemplate string, parameter interface{}) (bool, error)
	Status(templateContext map[string]interface{}, customStatusTemplate string, parameter interface{}) (string, error)
	GetTemplateContext(ctx process.Context, cli client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error)
}

type def struct {
	name string
	pd   *packages.PackageDiscover
}

type workloadDef struct {
	def
}

// NewWorkloadAbstractEngine create Workload Definition AbstractEngine
func NewWorkloadAbstractEngine(name string, pd *packages.PackageDiscover) AbstractEngine {
	return &workloadDef{
		def: def{
			name: name,
			pd:   pd,
		},
	}
}

// Complete do workload definition's rendering
func (wd *workloadDef) Complete(ctx process.Context, abstractTemplate string, params interface{}) error {
	bi := build.NewContext().NewInstance("", nil)
	if err := value.AddFile(bi, "-", renderTemplate(abstractTemplate)); err != nil {
		return errors.WithMessagef(err, "invalid cue template of workload %s", wd.name)
	}
	var paramFile = velaprocess.ParameterFieldName + ": {}"
	if params != nil {
		bt, err := json.Marshal(params)
		if err != nil {
			return errors.WithMessagef(err, "marshal parameter of workload %s", wd.name)
		}
		if string(bt) != "null" {
			paramFile = fmt.Sprintf("%s: %s", velaprocess.ParameterFieldName, string(bt))
		}
	}
	if err := value.AddFile(bi, velaprocess.ParameterFieldName, paramFile); err != nil {
		return errors.WithMessagef(err, "invalid parameter of workload %s", wd.name)
	}

	c, err := ctx.BaseContextFile()
	if err != nil {
		return err
	}
	if err := value.AddFile(bi, "context", c); err != nil {
		return err
	}

	val, err := wd.pd.ImportPackagesAndBuildValue(bi)
	if err != nil {
		return err
	}

	if err := val.Validate(); err != nil {
		return errors.WithMessagef(err, "invalid cue template of workload %s after merge parameter and context", wd.name)
	}
	output := val.LookupPath(value.FieldPath(OutputFieldName))
	base, err := model.NewBase(output)
	if err != nil {
		return errors.WithMessagef(err, "invalid output of workload %s", wd.name)
	}
	if err := ctx.SetBase(base); err != nil {
		return err
	}

	// we will support outputs for workload composition, and it will become trait in AppConfig.
	outputs := val.LookupPath(value.FieldPath(OutputsFieldName))
	if !outputs.Exists() {
		return nil
	}
	iter, err := outputs.Fields(cue.Definitions(true), cue.Hidden(true), cue.All())
	if err != nil {
		return errors.WithMessagef(err, "invalid outputs of workload %s", wd.name)
	}
	for iter.Next() {
		if iter.Selector().IsDefinition() || iter.Selector().PkgPath() != "" || iter.IsOptional() {
			continue
		}
		other, err := model.NewOther(iter.Value())
		name := iter.Label()
		if err != nil {
			return errors.WithMessagef(err, "invalid outputs(%s) of workload %s", name, wd.name)
		}
		if err := ctx.AppendAuxiliaries(process.Auxiliary{Ins: other, Type: AuxiliaryWorkload, Name: name}); err != nil {
			return err
		}
	}
	return nil
}

func (wd *workloadDef) getTemplateContext(ctx process.Context, cli client.Reader, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	baseLabels := GetBaseContextLabels(ctx)
	var root = initRoot(baseLabels)
	var commonLabels = GetCommonLabels(baseLabels)

	base, assists := ctx.Output()
	componentWorkload, err := base.Unstructured()
	if err != nil {
		return nil, err
	}
	// workload main resource will have a unique label("app.oam.dev/resourceType"="WORKLOAD") in per component/app level
	object, err := getResourceFromObj(ctx, componentWorkload, cli, accessor.For(componentWorkload), util.MergeMapOverrideWithDst(map[string]string{
		oam.LabelOAMResourceType: oam.ResourceTypeWorkload,
	}, commonLabels), "")
	if err != nil {
		return nil, err
	}
	root[OutputFieldName] = object
	outputs := make(map[string]interface{})
	for _, assist := range assists {
		if assist.Type != AuxiliaryWorkload {
			continue
		}
		if assist.Name == "" {
			return nil, errors.New("the auxiliary of workload must have a name with format 'outputs.<my-name>'")
		}
		traitRef, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, err
		}
		// AuxiliaryWorkload will have a unique label("trait.oam.dev/resource"="name of outputs") in per component/app level
		object, err := getResourceFromObj(ctx, traitRef, cli, accessor.For(traitRef), util.MergeMapOverrideWithDst(map[string]string{
			oam.TraitTypeLabel: AuxiliaryWorkload,
		}, commonLabels), assist.Name)
		if err != nil {
			return nil, err
		}
		outputs[assist.Name] = object
	}
	if len(outputs) > 0 {
		root[OutputsFieldName] = outputs
	}
	return root, nil
}

func formatRuntimeContext(templateContext map[string]interface{}, parameter interface{}) (string, error) {
	var paramBuff = "parameter: {}\n"

	bt, err := json.Marshal(templateContext)
	if err != nil {
		return "", errors.WithMessage(err, "json marshal template context")
	}
	ctxBuff := "context: " + string(bt) + "\n"

	bt, err = json.Marshal(parameter)
	if err != nil {
		return "", errors.WithMessage(err, "json marshal template parameters")
	}
	if string(bt) != "null" {
		paramBuff = "parameter: " + string(bt) + "\n"
	}
	return ctxBuff + paramBuff, nil
}

// HealthCheck address health check for workload
func (wd *workloadDef) HealthCheck(templateContext map[string]interface{}, healthPolicyTemplate string, parameter interface{}) (bool, error) {
	return checkHealth(templateContext, healthPolicyTemplate, parameter)
}

func checkHealth(templateContext map[string]interface{}, healthPolicyTemplate string, parameter interface{}) (bool, error) {
	if healthPolicyTemplate == "" {
		return true, nil
	}
	runtimeContextBuff, err := formatRuntimeContext(templateContext, parameter)
	if err != nil {
		return false, err
	}
	var buff = healthPolicyTemplate + "\n" + runtimeContextBuff

	val := cuecontext.New().CompileString(buff)
	healthy, err := val.LookupPath(value.FieldPath(HealthCheckPolicy)).Bool()
	if err != nil {
		return false, errors.WithMessage(err, "evaluate health status")
	}
	return healthy, nil
}

// Status get workload status by customStatusTemplate
func (wd *workloadDef) Status(templateContext map[string]interface{}, customStatusTemplate string, parameter interface{}) (string, error) {
	return getStatusMessage(wd.pd, templateContext, customStatusTemplate, parameter)
}

func getStatusMessage(pd *packages.PackageDiscover, templateContext map[string]interface{}, customStatusTemplate string, parameter interface{}) (string, error) {
	if customStatusTemplate == "" {
		return "", nil
	}
	runtimeContextBuff, err := formatRuntimeContext(templateContext, parameter)
	if err != nil {
		return "", err
	}
	var buff = customStatusTemplate + "\n" + runtimeContextBuff

	val, err := value.NewValue(buff, pd, "")
	if err != nil {
		return "", errors.WithMessage(err, "compile status template")
	}
	message, err := val.CueValue().LookupPath(value.FieldPath(CustomMessage)).String()
	if err != nil {
		return "", errors.WithMessage(err, "evaluate customStatus.message")
	}
	return message, nil
}

func (wd *workloadDef) GetTemplateContext(ctx process.Context, cli client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	return wd.getTemplateContext(ctx, cli, accessor)
}

type traitDef struct {
	def
}

// NewTraitAbstractEngine create Trait Definition AbstractEngine
func NewTraitAbstractEngine(name string, pd *packages.PackageDiscover) AbstractEngine {
	return &traitDef{
		def: def{
			name: name,
			pd:   pd,
		},
	}
}

// Complete do trait definition's rendering
// nolint:gocyclo
func (td *traitDef) Complete(ctx process.Context, abstractTemplate string, params interface{}) error {
	bi := build.NewContext().NewInstance("", nil)
	buff := abstractTemplate + "\n"
	if params != nil {
		bt, err := json.Marshal(params)
		if err != nil {
			return errors.WithMessagef(err, "marshal parameter of trait %s", td.name)
		}
		if string(bt) != "null" {
			buff += fmt.Sprintf("%s: %s\n", velaprocess.ParameterFieldName, string(bt))
		}
	}
	c, err := ctx.BaseContextFile()
	if err != nil {
		return err
	}
	buff += c
	if err := value.AddFile(bi, "-", buff); err != nil {
		return errors.WithMessagef(err, "invalid context of trait %s", td.name)
	}

	val, err := td.pd.ImportPackagesAndBuildValue(bi)
	if err != nil {
		return err
	}

	if err := val.Validate(); err != nil {
		return errors.WithMessagef(err, "invalid template of trait %s after merge with parameter and context", td.name)
	}
	processing := val.LookupPath(value.FieldPath("processing"))
	if processing.Exists() {
		if val, err = task.Process(val); err != nil {
			return errors.WithMessagef(err, "invalid process of trait %s", td.name)
		}
	}
	outputs := val.LookupPath(value.FieldPath(OutputsFieldName))
	if outputs.Exists() {
		iter, err := outputs.Fields(cue.Definitions(true), cue.Hidden(true), cue.All())
		if err != nil {
			return errors.WithMessagef(err, "invalid outputs of trait %s", td.name)
		}
		for iter.Next() {
			if iter.Selector().IsDefinition() || iter.Selector().PkgPath() != "" || iter.IsOptional() {
				continue
			}
			other, err := model.NewOther(iter.Value())
			name := iter.Label()
			if err != nil {
				return errors.WithMessagef(err, "invalid outputs(resource=%s) of trait %s", name, td.name)
			}
			if err := ctx.AppendAuxiliaries(process.Auxiliary{Ins: other, Type: td.name, Name: name}); err != nil {
				return err
			}
		}
	}

	patcher := val.LookupPath(value.FieldPath(PatchFieldName))
	base, auxiliaries := ctx.Output()
	if patcher.Exists() {
		if base == nil {
			return fmt.Errorf("patch trait %s into an invalid workload", td.name)
		}
		if err := base.Unify(patcher, sets.CreateUnifyOptionsForPatcher(patcher)...); err != nil {
			return errors.WithMessagef(err, "invalid patch trait %s into workload", td.name)
		}
	}
	outputsPatcher := val.LookupPath(value.FieldPath(PatchOutputsFieldName))
	if outputsPatcher.Exists() {
		for _, auxiliary := range auxiliaries {
			target := outputsPatcher.LookupPath(value.FieldPath(auxiliary.Name))
			if !target.Exists() {
				continue
			}
			if err = auxiliary.Ins.Unify(target); err != nil {
				return errors.WithMessagef(err, "trait=%s, to=%s, invalid patch trait into auxiliary workload", td.name, auxiliary.Name)
			}
		}
	}

	errs := val.LookupPath(value.FieldPath(ErrsFieldName))
	if errs.Exists() {
		if err := parseErrors(errs); err != nil {
			return err
		}
	}

	return nil
}

func parseErrors(errs cue.Value) error {
	if it, e := errs.List(); e == nil {
		for it.Next() {
			if s, err := it.Value().String(); err == nil && s != "" {
				return errors.Errorf(s)
			}
		}
	}
	return nil
}

// GetCommonLabels will convert context based labels to OAM standard labels
func GetCommonLabels(contextLabels map[string]string) map[string]string {
	var commonLabels = map[string]string{}
	for k, v := range contextLabels {
		switch k {
		case velaprocess.ContextAppName:
			commonLabels[oam.LabelAppName] = v
		case velaprocess.ContextName:
			commonLabels[oam.LabelAppComponent] = v
		case velaprocess.ContextAppRevision:
			commonLabels[oam.LabelAppRevision] = v
		case velaprocess.ContextReplicaKey:
			commonLabels[oam.LabelReplicaKey] = v

		}
	}
	return commonLabels
}

// GetBaseContextLabels get base context labels
func GetBaseContextLabels(ctx process.Context) map[string]string {
	baseLabels := ctx.BaseContextLabels()
	baseLabels[velaprocess.ContextAppName] = ctx.GetData(velaprocess.ContextAppName).(string)
	baseLabels[velaprocess.ContextAppRevision] = ctx.GetData(velaprocess.ContextAppRevision).(string)

	return baseLabels
}

func initRoot(contextLabels map[string]string) map[string]interface{} {
	var root = map[string]interface{}{}
	for k, v := range contextLabels {
		root[k] = v
	}
	return root
}

func renderTemplate(templ string) string {
	return templ + `
context: _
parameter: _
`
}

func (td *traitDef) getTemplateContext(ctx process.Context, cli client.Reader, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	baseLabels := GetBaseContextLabels(ctx)
	var root = initRoot(baseLabels)
	var commonLabels = GetCommonLabels(baseLabels)

	_, assists := ctx.Output()
	outputs := make(map[string]interface{})
	for _, assist := range assists {
		if assist.Type != td.name {
			continue
		}
		traitRef, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, err
		}
		object, err := getResourceFromObj(ctx, traitRef, cli, accessor.For(traitRef), util.MergeMapOverrideWithDst(map[string]string{
			oam.TraitTypeLabel: assist.Type,
		}, commonLabels), assist.Name)
		if err != nil {
			return nil, err
		}
		outputs[assist.Name] = object
	}
	if len(outputs) > 0 {
		root[OutputsFieldName] = outputs
	}
	return root, nil
}

// Status get trait status by customStatusTemplate
func (td *traitDef) Status(templateContext map[string]interface{}, customStatusTemplate string, parameter interface{}) (string, error) {
	return getStatusMessage(td.pd, templateContext, customStatusTemplate, parameter)
}

// HealthCheck address health check for trait
func (td *traitDef) HealthCheck(templateContext map[string]interface{}, healthPolicyTemplate string, parameter interface{}) (bool, error) {
	return checkHealth(templateContext, healthPolicyTemplate, parameter)
}

func (td *traitDef) GetTemplateContext(ctx process.Context, cli client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	return td.getTemplateContext(ctx, cli, accessor)
}

func getResourceFromObj(ctx process.Context, obj *unstructured.Unstructured, client client.Reader, namespace string, labels map[string]string, outputsResource string) (map[string]interface{}, error) {
	if outputsResource != "" {
		labels[oam.TraitResource] = outputsResource
	}
	if obj.GetName() != "" {
		u, err := util.GetObjectGivenGVKAndName(ctx.GetCtx(), client, obj.GroupVersionKind(), namespace, obj.GetName())
		if err != nil {
			return nil, err
		}
		return u.Object, nil
	}
	if ctxName := ctx.GetData(model.ContextName).(string); ctxName != "" {
		u, err := util.GetObjectGivenGVKAndName(ctx.GetCtx(), client, obj.GroupVersionKind(), namespace, ctxName)
		if err == nil {
			return u.Object, nil
		}
	}
	list, err := util.GetObjectsGivenGVKAndLabels(ctx.GetCtx(), client, obj.GroupVersionKind(), namespace, labels)
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 1 {
		return list.Items[0].Object, nil
	}
	for _, v := range list.Items {
		if v.GetLabels()[oam.TraitResource] == outputsResource {
			return v.Object, nil
		}
	}
	return nil, errors.Errorf("no resources found gvk(%v) labels(%v)", obj.GroupVersionKind(), labels)
}
