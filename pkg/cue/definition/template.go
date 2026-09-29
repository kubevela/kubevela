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
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/features"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"
	"github.com/kubevela/pkg/multicluster"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/cue/model"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/process"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/cue/render"
	"github.com/oam-dev/kubevela/pkg/cue/task"
	"github.com/oam-dev/kubevela/pkg/cue/upgrade"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/sources"
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
	// ErrsFieldName check if errors contained in the cue. Kept as an alias so
	// existing callers of definition.ErrsFieldName still compile; the value lives
	// in pkg/cue/render, shared with source resolution.
	ErrsFieldName = render.ErrsFieldName
	// TemplateContextPrefix is the base prefix for storing templates in context
	TemplateContextPrefix = "template-context-"
)

// GetWorkloadTemplateKey returns the context key for storing workload templates
func GetWorkloadTemplateKey(name string) string {
	return TemplateContextPrefix + "workload-" + name
}

// GetTraitTemplateKey returns the context key for storing trait templates
// GetTraitTemplateKey returns the context key for storing trait templates
func GetTraitTemplateKey(name string) string {
	return TemplateContextPrefix + "trait-" + name
}

const (
	// AuxiliaryWorkload defines the extra workload obj from a workloadDefinition,
	// e.g. a workload composed by deployment and service, the service will be marked as AuxiliaryWorkload
	AuxiliaryWorkload = "AuxiliaryWorkload"
)

// AbstractEngine defines Definition's Render interface
// AbstractEngine defines Definition's Render interface
type AbstractEngine interface {
	Complete(ctx process.Context, abstractTemplate string, params interface{}) error
	Status(templateContext map[string]interface{}, request *health.StatusRequest) (*health.StatusResult, error)
	GetTemplateContext(ctx process.Context, cli client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error)
}
type def struct {
	name string
	// ancestors are the definitions this one extends, nearest parent first.
	// Empty for a definition that extends nothing, which is the common case and
	// renders exactly as it always did.
	ancestors []inherit.Level
}
type workloadDef struct {
	def
	// surface is the call site this definition renders on. A PolicyDefinition
	// with a CUE template renders through this same engine, but it is not a
	// component: its readable context differs, and context.name is the policy.
	surface string
}

// NewWorkloadAbstractEngine create Workload Definition AbstractEngine.
//
// Ancestors are the definitions this one extends, nearest parent first. They
// are variadic so that every existing caller, and every definition that extends
// nothing, is unaffected.
func NewWorkloadAbstractEngine(name string, ancestors ...inherit.Level) AbstractEngine {
	return &workloadDef{
		def: def{
			name:      name,
			ancestors: ancestors,
		},
		surface: sources.SurfaceComponent,
	}
}

// NewPolicyAbstractEngine creates the engine for a PolicyDefinition that renders
// resources. Same machinery as a component, different surface - so its
// expressions are typed and resolved against the context a policy render has.
// NewPolicyAbstractEngine creates the engine for a PolicyDefinition that renders
// resources. Same machinery as a component, different surface - so its
// expressions are typed and resolved against the context a policy render has.
func NewPolicyAbstractEngine(name string) AbstractEngine {
	return &workloadDef{
		def:     def{name: name},
		surface: sources.SurfacePolicyRendered,
	}
}

// Complete do workload definition's rendering
// Complete do workload definition's rendering
func (wd *workloadDef) Complete(ctx process.Context, abstractTemplate string, params interface{}) (retErr error) {
	start := time.Now()
	defer func() {
		status := "ok"
		if retErr != nil {
			status = "error"
		}
		CUERenderDuration.WithLabelValues(string(upgrade.ComponentKind), status).Observe(time.Since(start).Seconds())
	}()

	var paramFile = velaprocess.ParameterFieldName + ": {}"
	if params != nil {
		surface := wd.surface
		if surface == "" {
			surface = sources.SurfaceComponent
		}
		resolved, err := sources.ResolveSourceExpressions(ctx, params, surface)
		if err != nil {
			return errors.WithMessagef(err, "resolve source expressions for %s %s", surface, wd.name)
		}
		bt, err := renderParams(ctx, params, resolved)
		if err != nil {
			return errors.WithMessagef(err, "marshal parameter of workload %s", wd.name)
		}
		if bt != "null" {
			paramFile = fmt.Sprintf("%s: %s", velaprocess.ParameterFieldName, bt)
		}
	}

	c, err := ctx.BaseContextFile()
	if err != nil {
		return err
	}

	abstractTemplate, _ = upgrade.EnsureCueVersionCompatibility(abstractTemplate, wd.name, upgrade.ComponentKind, upgrade.TemplateAreaMain)

	var val cue.Value
	var userErrors []string
	if wd.extendsSomething() {
		res, err := wd.renderChain(ctx, abstractTemplate, paramFile, c, inherit.ComponentSurface)
		if err != nil {
			return err
		}
		val, userErrors = res.value, res.userErrors
	} else {
		val, err = velacuex.WorkloadCompiler.Get().CompileString(ctx.GetCtx(), strings.Join([]string{
			render.Template(abstractTemplate), paramFile, c,
		}, "\n"))
		if err != nil {
			return errors.WithMessagef(err, "failed to compile workload %s after merge parameter and context", wd.name)
		}
		userErrors = render.UserErrors(val, "Workload definition", wd.name)
	}

	validationErr := val.Validate()

	if validationErr != nil || len(userErrors) > 0 {
		verr := &ValidationError{Kind: "workload", Name: wd.name, User: userErrors}
		verr.Parameter, verr.Template = cueErrorMessages(validationErr, &val)
		return verr
	}
	output := val.LookupPath(value.FieldPath(OutputFieldName))
	// A typed parameter leaves this non-concrete, and the trait renders against
	// it as JSON. Prune it here, where nothing checks it, rather than in the
	// parameters, where something does.
	output = concreteForRender(ctx, output)

	base, err := model.NewBase(output)
	if err != nil {
		return errors.WithMessagef(err, "invalid output of workload %s", wd.name)
	}
	if err := ctx.SetBase(base); err != nil {
		return err
	}

	// Store template for error context (use workload-specific key to avoid pollution)
	// Skipped during validation: the whole value is marshalled into every later
	// template's context, which a typed parameter cannot survive. The render
	// path falls back to the base when it is absent.
	if !sources.TypeOnly(ctx.GetCtx()) {
		ctx.PushData(GetWorkloadTemplateKey(wd.name), val)
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
		other, err := model.NewOther(concreteForRender(ctx, iter.Value()))
		name := util.GetIteratorLabel(*iter)
		if err != nil {
			return errors.WithMessagef(err, "invalid outputs(%s) of workload %s", name, wd.name)
		}
		if err := ctx.AppendAuxiliaries(process.Auxiliary{Ins: other, Type: AuxiliaryWorkload, Name: name}); err != nil {
			return err
		}
	}
	return nil
}
func withCluster(ctx context.Context, o client.Object) context.Context {
	if cluster := oam.GetCluster(o); cluster != "" {
		return multicluster.WithCluster(ctx, cluster)
	}
	return ctx
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
	_ctx := withCluster(ctx.GetCtx(), componentWorkload)
	object, err := getResourceFromObj(_ctx, ctx, componentWorkload, cli, accessor.For(componentWorkload), util.MergeMapOverrideWithDst(map[string]string{
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
		_ctx := withCluster(ctx.GetCtx(), traitRef)
		object, err := getResourceFromObj(_ctx, ctx, traitRef, cli, accessor.For(traitRef), util.MergeMapOverrideWithDst(map[string]string{
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

// Status get workload status by customStatusTemplate
// Status get workload status by customStatusTemplate
func (wd *workloadDef) Status(templateContext map[string]interface{}, request *health.StatusRequest) (*health.StatusResult, error) {
	return health.GetStatus(templateContext, request)
}
func (wd *workloadDef) GetTemplateContext(ctx process.Context, cli client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	return wd.getTemplateContext(ctx, cli, accessor)
}

type traitDef struct {
	def
}

// NewTraitAbstractEngine create Trait Definition AbstractEngine.
//
// Ancestors are the definitions this one extends, nearest parent first. They
// are variadic so that every existing caller, and every trait that extends
// nothing, is unaffected.
func NewTraitAbstractEngine(name string, ancestors ...inherit.Level) AbstractEngine {
	return &traitDef{
		def: def{
			name:      name,
			ancestors: ancestors,
		},
	}
}

// Complete do trait definition's rendering
// nolint:gocyclo
// Complete do trait definition's rendering
// nolint:gocyclo
func (td *traitDef) Complete(ctx process.Context, abstractTemplate string, params interface{}) (retErr error) {
	start := time.Now()
	defer func() {
		status := "ok"
		if retErr != nil {
			status = "error"
		}
		CUERenderDuration.WithLabelValues(string(upgrade.TraitKind), status).Observe(time.Since(start).Seconds())
	}()

	abstractTemplate, _ = upgrade.EnsureCueVersionCompatibility(abstractTemplate, td.name, upgrade.TraitKind, upgrade.TemplateAreaMain)

	var paramFile string
	if params != nil {
		resolved, err := sources.ResolveSourceExpressions(ctx, params, sources.SurfaceTrait)
		if err != nil {
			return errors.WithMessagef(err, "resolve source expressions for trait %s", td.name)
		}
		bt, err := renderParams(ctx, params, resolved)
		if err != nil {
			return errors.WithMessagef(err, "marshal parameter of trait %s", td.name)
		}
		if bt != "null" {
			paramFile = fmt.Sprintf("%s: %s\n", velaprocess.ParameterFieldName, bt)
		}
	}
	buff := abstractTemplate + "\n" + paramFile

	multiStageEnabled := feature.DefaultMutableFeatureGate.Enabled(features.MultiStageComponentApply)
	var statusBytes []byte
	if multiStageEnabled {
		statusBytes = outputStatusBytes(ctx)
	}

	c, err := ctx.BaseContextFile()
	if err != nil {
		return err
	}

	// When multi-stage is enabled, merge the existing output.status from ctx into the
	// base context so downstream CUE can reference it deterministically.
	if multiStageEnabled {
		c = injectOutputStatusIntoBaseContext(ctx, c, statusBytes)
	}

	buff += c

	var val cue.Value
	var userErrors []string
	var levels []cue.Value
	if td.extendsSomething() {
		res, err := td.renderChain(ctx, abstractTemplate, paramFile, c, inherit.TraitSurface)
		if err != nil {
			return err
		}
		val, userErrors, levels = res.value, res.userErrors, res.levels
	} else {
		val, err = velacuex.WorkloadCompiler.Get().CompileString(ctx.GetCtx(), buff)
		if err != nil {
			return errors.WithMessagef(err, "failed to compile trait %s after merge parameter and context", td.name)
		}
		userErrors = render.UserErrors(val, "Trait definition", td.name)
	}

	validationErr := val.Validate()

	if validationErr != nil || len(userErrors) > 0 {
		verr := &ValidationError{Kind: "trait", Name: td.name, User: userErrors}
		verr.Parameter, verr.Template = cueErrorMessages(validationErr, &val)
		return verr
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
			other, err := model.NewOther(concreteForRender(ctx, iter.Value()))
			name := util.GetIteratorLabel(*iter)
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
		if err := base.Unify(patcher, patchOptionsFromLevels(levels, patcher)...); err != nil {
			return errors.WithMessagef(err, "invalid patch trait %s into workload", td.name)
		}
		// The patch carries the trait's own parameters, so a source-fed one
		// lands here as a type. Pruned after the unification rather than before
		// it, because the patcher's attributes are what drive patch strategy.
		if err := repruneBase(ctx, base); err != nil {
			return errors.WithMessagef(err, "invalid patch trait %s into workload", td.name)
		}
	}
	outputsPatcher := val.LookupPath(value.FieldPath(PatchOutputsFieldName))
	if outputsPatcher.Exists() {
		for i, auxiliary := range auxiliaries {
			target := outputsPatcher.LookupPath(value.FieldPath(auxiliary.Name))
			if !target.Exists() {
				continue
			}
			if err = auxiliary.Ins.Unify(target); err != nil {
				return errors.WithMessagef(err, "trait=%s, to=%s, invalid patch trait into auxiliary workload", td.name, auxiliary.Name)
			}
			if err = repruneAuxiliary(ctx, auxiliaries, i); err != nil {
				return errors.WithMessagef(err, "trait=%s, to=%s, invalid patch trait into auxiliary workload", td.name, auxiliary.Name)
			}
		}
	}

	return nil
}
func outputStatusBytes(ctx process.Context) []byte {
	var statusBytes []byte
	var outputMap map[string]interface{}
	if output := ctx.GetData(OutputFieldName); output != nil {
		if m, ok := output.(map[string]interface{}); ok {
			outputMap = m
		} else if ptr, ok := output.(*interface{}); ok && ptr != nil {
			if m, ok := (*ptr).(map[string]interface{}); ok {
				outputMap = m
			}
		}

		if outputMap != nil {
			if status, ok := outputMap["status"]; ok {
				if b, err := json.Marshal(status); err == nil {
					statusBytes = b
				}
			}
		}
	}
	return statusBytes
}
func injectOutputStatusIntoBaseContext(ctx process.Context, c string, statusBytes []byte) string {
	if len(statusBytes) > 0 {
		// If output is an empty object, replace it with only the status field without trailing comma.
		emptyOutputMarker := "\"output\":{}"
		if strings.Contains(c, emptyOutputMarker) {
			replacement := fmt.Sprintf("\"output\":{\"status\":%s}", string(statusBytes))
			c = strings.Replace(c, emptyOutputMarker, replacement, 1)
		} else {
			// Otherwise, insert status as the first field and keep the comma to separate from existing fields.
			replacement := fmt.Sprintf("\"output\":{\"status\":%s,", string(statusBytes))
			c = strings.Replace(c, "\"output\":{", replacement, 1)
		}

		// Restore the status field to the current output in ctx.data
		var status interface{}
		if err := json.Unmarshal(statusBytes, &status); err == nil {
			if currentOutput := ctx.GetData(OutputFieldName); currentOutput != nil {
				if currentMap, ok := currentOutput.(map[string]interface{}); ok {
					currentMap["status"] = status
					ctx.PushData(OutputFieldName, currentMap)
				}
			}
		}
	}
	return c
}

// GetCommonLabels will convert context based labels to OAM standard labels
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
		_ctx := withCluster(ctx.GetCtx(), traitRef)
		object, err := getResourceFromObj(_ctx, ctx, traitRef, cli, accessor.For(traitRef), util.MergeMapOverrideWithDst(map[string]string{
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
// Status get trait status by customStatusTemplate
func (td *traitDef) Status(templateContext map[string]interface{}, request *health.StatusRequest) (*health.StatusResult, error) {
	return health.GetStatus(templateContext, request)
}
func (td *traitDef) GetTemplateContext(ctx process.Context, cli client.Client, accessor util.NamespaceAccessor) (map[string]interface{}, error) {
	return td.getTemplateContext(ctx, cli, accessor)
}
func getResourceFromObj(ctx context.Context, pctx process.Context, obj *unstructured.Unstructured, client client.Reader, namespace string, labels map[string]string, outputsResource string) (map[string]interface{}, error) {
	if outputsResource != "" {
		labels[oam.TraitResource] = outputsResource
	}
	if obj.GetName() != "" {
		u, err := util.GetObjectGivenGVKAndName(ctx, client, obj.GroupVersionKind(), namespace, obj.GetName())
		if err != nil {
			return nil, err
		}
		return u.Object, nil
	}
	if ctxName := pctx.GetData(model.ContextName).(string); ctxName != "" {
		u, err := util.GetObjectGivenGVKAndName(ctx, client, obj.GroupVersionKind(), namespace, ctxName)
		if err == nil {
			return u.Object, nil
		}
	}
	list, err := util.GetObjectsGivenGVKAndLabels(ctx, client, obj.GroupVersionKind(), namespace, labels)
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

// FormatCUEError returns err as a *ValidationError, its messages grouped into
// Parameter and Template sections, or nil when there is nothing to report.
func FormatCUEError(err error, messagePrefix string, entityType, entityName string, val ...*cue.Value) error {
	paramErrs, templateErrs := cueErrorMessages(err, val...)
	if len(paramErrs) == 0 && len(templateErrs) == 0 {
		return nil
	}
	return &ValidationError{
		Kind:      entityType,
		Name:      entityName,
		Parameter: paramErrs,
		Template:  templateErrs,
		header:    fmt.Sprintf("%s %s %s:", messagePrefix, entityType, entityName),
	}
}

// cueErrorMessages splits a CUE error, and the incomplete values left in
// val, into parameter and template messages, deduplicated and sorted.
func cueErrorMessages(err error, val ...*cue.Value) (params, templates []string) {
	if err == nil {
		return nil, nil
	}
	paramSet, templateSet := map[string]bool{}, map[string]bool{}
	collect := func(err error) {
		for _, e := range cueerrors.Errors(err) {
			if msg := e.Error(); strings.HasPrefix(msg, "parameter.") {
				paramSet[msg] = true
			} else {
				templateSet[msg] = true
			}
		}
	}
	collect(err)
	if len(val) > 0 && val[0] != nil {
		if concreteErr := val[0].Validate(cue.Concrete(true)); concreteErr != nil {
			collect(concreteErr)
		}
	}
	return sortedKeys(paramSet), sortedKeys(templateSet)
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func writeErrorSection(b *strings.Builder, title string, errs []string) {
	if len(errs) == 0 {
		return
	}
	b.WriteString("\n\n" + title + ":\n")
	for _, e := range errs {
		b.WriteString("  " + e + "\n")
	}
}

// ValidationError is a definition that failed validation once rendered: the
// errors it raised itself through `errs`, and the CUE errors in its parameters
// and in the rest of its template.
type ValidationError struct {
	// Kind is what failed: a workload, trait, component and so on.
	Kind      string
	Name      string
	User      []string
	Parameter []string
	Template  []string

	// header opens the message in place of "validation failed for ...".
	header string
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	if e.header != "" {
		b.WriteString(e.header)
	} else {
		b.WriteString(fmt.Sprintf("validation failed for %s %s:", e.Kind, e.Name))
	}
	writeErrorSection(&b, "User Errors", e.User)
	writeErrorSection(&b, "Parameter errors", e.Parameter)
	writeErrorSection(&b, "Template errors", e.Template)
	return strings.TrimRight(b.String(), "\n")
}

// renderParams writes a component or trait's properties as CUE. A validation
// renders types rather than values, and a type cannot survive json.Marshal.
func renderParams(ctx process.Context, params, resolved interface{}) (string, error) {
	chosen := params
	if resolved != nil {
		chosen = resolved
	}
	if typed, ok := chosen.(map[string]interface{}); ok && sources.TypeOnly(ctx.GetCtx()) {
		return sources.ParamsAsCUE(typed)
	}
	raw, err := json.Marshal(chosen)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// concreteForRender makes a validation's rendered resource marshalable. Every
// resource here is handed to the next template through the context as JSON,
// which an unknowable leaf cannot survive. A real render has nothing to prune.
func concreteForRender(ctx process.Context, v cue.Value) cue.Value {
	if !sources.TypeOnly(ctx.GetCtx()) {
		return v
	}
	pruned, _ := sources.ConcreteForValidation(v)
	return pruned
}

// repruneBase prunes a base that a patch has just made non-concrete again, and
// puts the result back so the next trait can be handed it as JSON.
func repruneBase(ctx process.Context, base model.Instance) error {
	if !sources.TypeOnly(ctx.GetCtx()) {
		return nil
	}
	pruned, changed := sources.ConcreteForValidation(base.Value())
	if !changed {
		return nil
	}
	next, err := model.NewBase(pruned)
	if err != nil {
		return err
	}
	return ctx.SetBase(next)
}

// repruneAuxiliary does the same for the i'th auxiliary. Unifying it with its
// pruned self would keep the open leaf, so the instance is replaced; Output
// hands back the context's own slice, which is what makes the replacement stick.
func repruneAuxiliary(ctx process.Context, auxiliaries []process.Auxiliary, i int) error {
	if !sources.TypeOnly(ctx.GetCtx()) {
		return nil
	}
	pruned, changed := sources.ConcreteForValidation(auxiliaries[i].Ins.Value())
	if !changed {
		return nil
	}
	next, err := model.NewOther(pruned)
	if err != nil {
		return err
	}
	auxiliaries[i].Ins = next
	return nil
}
