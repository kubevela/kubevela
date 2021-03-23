/*
Copyright 2020 The Crossplane Authors.

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

package applicationconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	types2 "github.com/oam-dev/kubevela/apis/types"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Render error strings.
const (
	errUnmarshalWorkload = "cannot unmarshal workload"
	errUnmarshalTrait    = "cannot unmarshal trait"
)

// Render error format strings.
const (
	errFmtGetComponent     = "cannot get component %q"
	errFmtGetScope         = "cannot get scope %q"
	errFmtResolveParams    = "cannot resolve parameter values for component %q"
	errFmtRenderWorkload   = "cannot render workload for component %q"
	errFmtRenderTrait      = "cannot render trait for component %q"
	errFmtSetParam         = "cannot set parameter %q"
	errFmtUnsupportedParam = "unsupported parameter %q"
	errFmtRequiredParam    = "required parameter %q not specified"
	errFmtCompRevision     = "cannot get latest revision for component %q while revision is enabled"
)

var (
	// ErrDataOutputNotExist is an error indicating the DataOutput specified doesn't not exist
	ErrDataOutputNotExist = errors.New("DataOutput does not exist")
)

// A ComponentRenderer renders an ApplicationConfiguration's Components into
// workloads and traits.
type ComponentRenderer interface {
	Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error)
}

// A ComponentRenderFn renders an ApplicationConfiguration's Components into
// workloads and traits.
type ComponentRenderFn func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error)

// Render an ApplicationConfiguration's Components into workloads and traits.
func (fn ComponentRenderFn) Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
	return fn(ctx, ac)
}

var _ ComponentRenderer = &components{}

type components struct {
	// indicate that if this is generated by application
	client   client.Reader
	dm       discoverymapper.DiscoveryMapper
	params   ParameterResolver
	workload ResourceRenderer
	trait    ResourceRenderer
}

func (r *components) Render(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
	workloads := make([]*Workload, 0, len(ac.Spec.Components))
	dag := newDAG()

	// we have special logics for application generated applicationConfiguration during rolling out phase
	rollingComponents := make(map[string]bool)
	var needRolloutTemplate bool
	if _, isAppRolling := ac.GetAnnotations()[oam.AnnotationAppRollout]; isAppRolling {
		// we only care about the new components when there is rolling out
		if anc, exist := ac.GetAnnotations()[oam.AnnotationRollingComponent]; exist {
			// the rolling components annotation contains all the rolling components in the application
			for _, componentName := range strings.Split(anc, common.RollingComponentsSep) {
				rollingComponents[componentName] = true
			}
		}
		// we need to do a template roll out if it's not done yet
		needRolloutTemplate = ac.Status.RollingStatus != types2.RollingTemplated
	} else if ac.Status.RollingStatus == types2.RollingTemplated {
		klog.InfoS("mark the ac rolling status as completed", "appConfig", klog.KRef(ac.Namespace, ac.Name))
		ac.Status.RollingStatus = types2.RollingCompleted
	}

	for _, acc := range ac.Spec.Components {
		if acc.RevisionName != "" {
			acc.ComponentName = utils.ExtractComponentName(acc.RevisionName)
		}
		isComponentRolling := rollingComponents[acc.ComponentName]
		w, err := r.renderComponent(ctx, acc, ac, isControlledByApp(ac), isComponentRolling, needRolloutTemplate, dag)
		if err != nil {
			return nil, nil, err
		}
		workloads = append(workloads, w)
		if isComponentRolling && needRolloutTemplate {
			ac.Status.RollingStatus = types2.RollingTemplating
		}
	}
	workloadsAllClear := true
	ds := &v1alpha2.DependencyStatus{}
	res := make([]Workload, 0, len(ac.Spec.Components))
	for i, acc := range ac.Spec.Components {
		unsatisfied, err := r.handleDependency(ctx, workloads[i], acc, dag, ac)
		if err != nil {
			return nil, nil, err
		}
		ds.Unsatisfied = append(ds.Unsatisfied, unsatisfied...)
		if workloads[i].HasDep {
			workloadsAllClear = false
		}
		res = append(res, *workloads[i])
	}
	// set the ac rollingStatus to be RollingTemplated if all workloads are going to be applied
	if workloadsAllClear && ac.Status.RollingStatus == types2.RollingTemplating {
		klog.InfoS("mark the ac rolling status as templated", "appConfig", klog.KRef(ac.Namespace, ac.Name))
		ac.Status.RollingStatus = types2.RollingTemplated
	}

	return res, ds, nil
}

func (r *components) renderComponent(ctx context.Context, acc v1alpha2.ApplicationConfigurationComponent,
	ac *v1alpha2.ApplicationConfiguration, isControlledByApp, isComponentRolling, needRolloutTemplate bool, dag *dag) (*Workload, error) {
	if acc.RevisionName != "" {
		acc.ComponentName = utils.ExtractComponentName(acc.RevisionName)
	}
	klog.InfoS("render a component", "component name", acc.ComponentName, "component revision", acc.RevisionName,
		"is generated by application", isControlledByApp, "is the component rolling", isComponentRolling,
		"is the appConfig a rollout template", needRolloutTemplate)

	c, componentRevisionName, err := util.GetComponent(ctx, r.client, acc, ac.GetNamespace())
	if err != nil {
		return nil, err
	}
	p, err := r.params.Resolve(c.Spec.Parameters, acc.ParameterValues)
	if err != nil {
		return nil, errors.Wrapf(err, errFmtResolveParams, acc.ComponentName)
	}

	w, err := r.workload.Render(c.Spec.Workload.Raw, p...)
	if err != nil {
		return nil, errors.Wrapf(err, errFmtRenderWorkload, acc.ComponentName)
	}

	compInfoLabels := map[string]string{
		oam.LabelAppName:              ac.Name,
		oam.LabelAppComponent:         acc.ComponentName,
		oam.LabelAppComponentRevision: componentRevisionName,
		oam.LabelOAMResourceType:      oam.ResourceTypeWorkload,
	}
	util.AddLabels(w, compInfoLabels)

	compInfoAnnotations := map[string]string{
		oam.AnnotationAppGeneration: strconv.Itoa(int(ac.Generation)),
	}
	util.AddAnnotations(w, compInfoAnnotations)

	// pass through labels and annotation from app-config to workload
	util.PassLabelAndAnnotation(ac, w)
	// don't pass the following annotation as those are for appConfig only
	util.RemoveAnnotations(w, []string{oam.AnnotationAppRollout, oam.AnnotationRollingComponent})
	ref := metav1.NewControllerRef(ac, v1alpha2.ApplicationConfigurationGroupVersionKind)
	w.SetNamespace(ac.GetNamespace())

	traits := make([]*Trait, 0, len(acc.Traits))
	traitDefs := make([]v1alpha2.TraitDefinition, 0, len(acc.Traits))
	compInfoLabels[oam.LabelOAMResourceType] = oam.ResourceTypeTrait

	for _, ct := range acc.Traits {
		t, traitDef, err := r.renderTrait(ctx, ct, ac, acc.ComponentName, ref, dag)
		if err != nil {
			return nil, err
		}
		util.AddLabels(t, compInfoLabels)
		util.AddAnnotations(t, compInfoAnnotations)

		// pass through labels and annotation from app-config to trait
		util.PassLabelAndAnnotation(ac, t)
		util.RemoveAnnotations(t, []string{oam.AnnotationAppRollout, oam.AnnotationRollingComponent})
		traits = append(traits, &Trait{Object: *t, Definition: *traitDef})
		traitDefs = append(traitDefs, *traitDef)
	}
	if !isControlledByApp {
		// This is the legacy standalone appConfig approach
		existingWorkload, err := r.getExistingWorkload(ctx, ac, c, w)
		if err != nil {
			return nil, err
		}
		if err := setWorkloadInstanceName(traitDefs, w, c, existingWorkload); err != nil {
			return nil, err
		}
	} else {
		// we have completely different approaches on workload name for application generated appConfig
		if c.Spec.Helm != nil {
			// for helm workload, make sure the workload is already generated by Helm successfully
			existingWorkloadByHelm, err := discoverHelmModuleWorkload(ctx, r.client, c, ac.GetNamespace())
			if err != nil {
				klog.ErrorS(err, "Could not get the workload created by Helm module",
					"component name", acc.ComponentName, "component revision", acc.RevisionName)
				return nil, errors.Wrap(err, "cannot get the workload created by a Helm module")
			}
			klog.InfoS("Successfully discovered the workload created by Helm",
				"component name", acc.ComponentName, "component revision", acc.RevisionName,
				"workload name", existingWorkloadByHelm.GetName())
			// use the name already generated instead of setting a new one
			w.SetName(existingWorkloadByHelm.GetName())
		} else {
			// for non-helm workload, we generate a workload name based on component name and revision
			revision, err := utils.ExtractRevision(acc.RevisionName)
			if err != nil {
				return nil, err
			}
			SetAppWorkloadInstanceName(acc.ComponentName, w, revision)
			if isComponentRolling && needRolloutTemplate {
				// we have a special logic to emit the workload as a template so that the rollout
				// controller can take over.
				// TODO: We might need to add the owner reference to the existing object in case the resource
				// is going to be shared (ie. CloneSet)
				if err := prepWorkloadInstanceForRollout(w); err != nil {
					return nil, err
				}
				// yield the controller to the rollout
				ref.Controller = pointer.BoolPtr(false)
				klog.InfoS("Successfully rendered a workload instance for rollout", "workload", w.GetName())
			}
		}
	}
	// set the owner reference after its ref is edited
	w.SetOwnerReferences([]metav1.OwnerReference{*ref})

	// create the ref after the workload name is set
	workloadRef := runtimev1alpha1.TypedReference{
		APIVersion: w.GetAPIVersion(),
		Kind:       w.GetKind(),
		Name:       w.GetName(),
	}
	//  We only patch a TypedReference object to the trait if it asks for it
	for i := range acc.Traits {
		traitDef := traitDefs[i]
		trait := traits[i]
		workloadRefPath := traitDef.Spec.WorkloadRefPath
		if len(workloadRefPath) != 0 {
			if err := fieldpath.Pave(trait.Object.UnstructuredContent()).SetValue(workloadRefPath, workloadRef); err != nil {
				return nil, errors.Wrapf(err, errFmtSetWorkloadRef, trait.Object.GetName(), w.GetName())
			}
		}
	}
	scopes := make([]unstructured.Unstructured, 0, len(acc.Scopes))
	for _, cs := range acc.Scopes {
		scopeObject, err := r.renderScope(ctx, cs, ac.GetNamespace())
		if err != nil {
			return nil, err
		}

		scopes = append(scopes, *scopeObject)
	}

	addDataOutputsToDAG(dag, acc.DataOutputs, w)
	// To avoid conflict with rollout controller, we will not render the workload until the rollout phase is over
	// indicated by the AnnotationAppRollout annotation disappear
	return &Workload{ComponentName: acc.ComponentName, ComponentRevisionName: componentRevisionName,
		SkipApply: isComponentRolling && !needRolloutTemplate,
		Workload:  w, Traits: traits, RevisionEnabled: isRevisionEnabled(traitDefs), Scopes: scopes}, nil
}

func (r *components) renderTrait(ctx context.Context, ct v1alpha2.ComponentTrait, ac *v1alpha2.ApplicationConfiguration,
	componentName string, ref *metav1.OwnerReference, dag *dag) (*unstructured.Unstructured, *v1alpha2.TraitDefinition, error) {
	t, err := r.trait.Render(ct.Trait.Raw)
	if err != nil {
		return nil, nil, errors.Wrapf(err, errFmtRenderTrait, componentName)
	}
	traitDef, err := util.FetchTraitDefinition(ctx, r.client, r.dm, t)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return nil, nil, errors.Wrapf(err, errFmtGetTraitDefinition, t.GetAPIVersion(), t.GetKind(), t.GetName())
		}
		traitDef = util.GetDummyTraitDefinition(t)
	}

	traitName := getTraitName(ac, componentName, &ct, t, traitDef)
	setTraitProperties(t, traitName, ac.GetNamespace(), ref)

	addDataOutputsToDAG(dag, ct.DataOutputs, t)

	return t, traitDef, nil
}

func (r *components) renderScope(ctx context.Context, cs v1alpha2.ComponentScope, ns string) (*unstructured.Unstructured, error) {
	// Get Scope instance from k8s, since it is global and not a child resource of workflow.
	scopeObject := &unstructured.Unstructured{}
	scopeObject.SetAPIVersion(cs.ScopeReference.APIVersion)
	scopeObject.SetKind(cs.ScopeReference.Kind)
	scopeObjectRef := types.NamespacedName{Namespace: ns, Name: cs.ScopeReference.Name}
	if err := r.client.Get(ctx, scopeObjectRef, scopeObject); err != nil {
		return nil, errors.Wrapf(err, errFmtGetScope, cs.ScopeReference.Name)
	}
	return scopeObject, nil
}

func setTraitProperties(t *unstructured.Unstructured, traitName, namespace string, ref *metav1.OwnerReference) {
	// Set metadata name for `Trait` if the metadata name is NOT set.
	if t.GetName() == "" {
		t.SetName(traitName)
	}

	t.SetOwnerReferences([]metav1.OwnerReference{*ref})
	t.SetNamespace(namespace)
}

// setWorkloadInstanceName will set metadata.name for workload CR according to createRevision flag in traitDefinition
func setWorkloadInstanceName(traitDefs []v1alpha2.TraitDefinition, w *unstructured.Unstructured,
	c *v1alpha2.Component, existingWorkload *unstructured.Unstructured) error {
	// Don't override the specified name
	if w.GetName() != "" {
		return nil
	}
	// TODO: revisit this logic
	// the name of the workload should depend on the workload type and if we are rolling or replacing upgrade
	// i.e Cloneset type of workload just use the component name while deployment type of workload will have revision
	// if we are doing rolling upgrades. We can just override if we are replacing the deployment.
	if isRevisionEnabled(traitDefs) {
		if c.Status.LatestRevision == nil {
			return fmt.Errorf(errFmtCompRevision, c.Name)
		}

		componentLastRevision := c.Status.LatestRevision.Name
		// if workload exists, check the revision label, we will not change the name if workload exists and no revision changed
		if existingWorkload != nil && existingWorkload.GetLabels()[oam.LabelAppComponentRevision] == componentLastRevision {
			// using the existing name
			w.SetName(existingWorkload.GetName())
			return nil
		}

		// if revisionEnabled and the running workload's revision isn't equal to the component's latest reversion,
		// use revisionName as the workload name
		w.SetName(componentLastRevision)
		return nil
	}
	// use component name as workload name, which means we will always use one workload for different revisions
	w.SetName(c.GetName())
	return nil
}

// isRevisionEnabled will check if any of the traitDefinitions has a createRevision flag
func isRevisionEnabled(traitDefs []v1alpha2.TraitDefinition) bool {
	for _, td := range traitDefs {
		if td.Spec.RevisionEnabled {
			return true
		}
	}
	return false
}

// A ResourceRenderer renders a Kubernetes-compliant YAML resource into an
// Unstructured object, optionally setting the supplied parameters.
type ResourceRenderer interface {
	Render(data []byte, p ...Parameter) (*unstructured.Unstructured, error)
}

// A ResourceRenderFn renders a Kubernetes-compliant YAML resource into an
// Unstructured object, optionally setting the supplied parameters.
type ResourceRenderFn func(data []byte, p ...Parameter) (*unstructured.Unstructured, error)

// Render the supplied Kubernetes YAML resource.
func (fn ResourceRenderFn) Render(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
	return fn(data, p...)
}

func renderWorkload(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
	// TODO(negz): Is there a better decoder to use here?
	w := &fieldpath.Paved{}
	if err := json.Unmarshal(data, w); err != nil {
		return nil, errors.Wrap(err, errUnmarshalWorkload)
	}

	for _, param := range p {
		for _, path := range param.FieldPaths {
			// TODO(negz): Infer parameter type from workload OpenAPI schema.
			switch param.Value.Type {
			case intstr.String:
				if err := w.SetString(path, param.Value.StrVal); err != nil {
					return nil, errors.Wrapf(err, errFmtSetParam, param.Name)
				}
			case intstr.Int:
				if err := w.SetNumber(path, float64(param.Value.IntVal)); err != nil {
					return nil, errors.Wrapf(err, errFmtSetParam, param.Name)
				}
			}
		}
	}

	return &unstructured.Unstructured{Object: w.UnstructuredContent()}, nil
}

func renderTrait(data []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
	// TODO(negz): Is there a better decoder to use here?
	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, u); err != nil {
		return nil, errors.Wrap(err, errUnmarshalTrait)
	}
	return u, nil
}

// A Parameter may be used to set the supplied paths to the supplied value.
type Parameter struct {
	// Name of this parameter.
	Name string

	// Value of this parameter.
	Value intstr.IntOrString

	// FieldPaths that should be set to this parameter's value.
	FieldPaths []string
}

// A ParameterResolver resolves the parameters accepted by a component and the
// parameter values supplied to a component into configured parameters.
type ParameterResolver interface {
	Resolve([]v1alpha2.ComponentParameter, []v1alpha2.ComponentParameterValue) ([]Parameter, error)
}

// A ParameterResolveFn resolves the parameters accepted by a component and the
// parameter values supplied to a component into configured parameters.
type ParameterResolveFn func([]v1alpha2.ComponentParameter, []v1alpha2.ComponentParameterValue) ([]Parameter, error)

// Resolve the supplied parameters.
func (fn ParameterResolveFn) Resolve(cp []v1alpha2.ComponentParameter, cpv []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
	return fn(cp, cpv)
}

func resolve(cp []v1alpha2.ComponentParameter, cpv []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
	supported := make(map[string]bool)
	for _, v := range cp {
		supported[v.Name] = true
	}

	set := make(map[string]*Parameter)
	for _, v := range cpv {
		if !supported[v.Name] {
			return nil, errors.Errorf(errFmtUnsupportedParam, v.Name)
		}
		set[v.Name] = &Parameter{Name: v.Name, Value: v.Value}
	}

	for _, p := range cp {
		_, ok := set[p.Name]
		if !ok && p.Required != nil && *p.Required {
			// This parameter is required, but not set.
			return nil, errors.Errorf(errFmtRequiredParam, p.Name)
		}
		if !ok {
			// This parameter is not required, and not set.
			continue
		}

		set[p.Name].FieldPaths = p.FieldPaths
	}

	params := make([]Parameter, 0, len(set))
	for _, p := range set {
		params = append(params, *p)
	}

	return params, nil
}

func addDataOutputsToDAG(dag *dag, outs []v1alpha2.DataOutput, obj *unstructured.Unstructured) {
	for _, out := range outs {
		r := &corev1.ObjectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			FieldPath:  out.FieldPath,
		}
		dag.AddSource(out.Name, r, out.Conditions)
	}
}

func (r *components) handleDependency(ctx context.Context, w *Workload, acc v1alpha2.ApplicationConfigurationComponent, dag *dag, ac *v1alpha2.ApplicationConfiguration) ([]v1alpha2.UnstaifiedDependency, error) {
	uds := make([]v1alpha2.UnstaifiedDependency, 0)
	unstructuredAC, err := util.Object2Unstructured(ac)
	if err != nil {
		return nil, errors.Wrapf(err, "handleDataInput by convert AppConfig (%s) to unstructured object failed", ac.Name)
	}
	// Record the dataOutput with ready conditions
	var unsatisfied []v1alpha2.UnstaifiedDependency
	unsatisfied, w.DataOutputs = r.handleDataOutput(ctx, acc.DataOutputs, dag, unstructuredAC)
	if len(unsatisfied) != 0 {
		uds = append(uds, unsatisfied...)
	}
	unsatisfied, err = r.handleDataInput(ctx, acc.DataInputs, dag, w.Workload, unstructuredAC)
	if err != nil {
		return nil, errors.Wrapf(err, "handleDataInput for workload (%s/%s) failed", w.Workload.GetNamespace(), w.Workload.GetName())
	}
	if len(unsatisfied) != 0 {
		uds = append(uds, unsatisfied...)
		w.HasDep = true
	} else {
		w.DataInputs = acc.DataInputs
	}

	for i, ct := range acc.Traits {
		trait := w.Traits[i]
		unsatisfied, trait.DataOutputs = r.handleDataOutput(ctx, ct.DataOutputs, dag, unstructuredAC)
		if len(unsatisfied) != 0 {
			uds = append(uds, unsatisfied...)
		}
		unsatisfied, err := r.handleDataInput(ctx, ct.DataInputs, dag, &trait.Object, unstructuredAC)
		if err != nil {
			return nil, errors.Wrapf(err, "handleDataInput for trait (%s/%s) failed", trait.Object.GetNamespace(), trait.Object.GetName())
		}
		if len(unsatisfied) != 0 {
			uds = append(uds, unsatisfied...)
			trait.HasDep = true
		} else {
			trait.DataInputs = ct.DataInputs
		}
	}
	return uds, nil
}

func makeUnsatisfiedDependency(obj *unstructured.Unstructured, s *dagSource, toPaths []string, reason string) v1alpha2.UnstaifiedDependency {
	return v1alpha2.UnstaifiedDependency{
		Reason: reason,
		From: v1alpha2.DependencyFromObject{
			TypedReference: runtimev1alpha1.TypedReference{
				APIVersion: s.ObjectRef.APIVersion,
				Kind:       s.ObjectRef.Kind,
				Name:       s.ObjectRef.Name,
			},
			FieldPath: s.ObjectRef.FieldPath,
		},
		To: v1alpha2.DependencyToObject{
			TypedReference: runtimev1alpha1.TypedReference{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
				Name:       obj.GetName(),
			},
			FieldPaths: toPaths,
		},
	}
}

func (r *components) handleDataOutput(ctx context.Context, outputs []v1alpha2.DataOutput, dag *dag, ac *unstructured.Unstructured) ([]v1alpha2.UnstaifiedDependency, map[string]v1alpha2.DataOutput) {
	uds := make([]v1alpha2.UnstaifiedDependency, 0)
	outputMap := make(map[string]v1alpha2.DataOutput)
	for _, out := range outputs {
		if reflect.DeepEqual(out.OutputStore, v1alpha2.StoreReference{}) {
			continue
		}
		s, ok := dag.Sources[out.Name]
		if !ok {
			continue
		}
		// the outputStore is considered ready when all conditions are ready
		allConditionsReady := true
		for _, oper := range out.OutputStore.Operations {
			newS := &dagSource{
				ObjectRef: &corev1.ObjectReference{
					APIVersion: s.ObjectRef.APIVersion,
					Kind:       s.ObjectRef.Kind,
					Name:       s.ObjectRef.Name,
					Namespace:  ac.GetNamespace(),
					FieldPath:  oper.ValueFrom.FieldPath,
				},
				Conditions: oper.Conditions,
			}
			_, ready, reason, err := r.getDataInput(ctx, newS, ac, false)
			if err != nil || !ready {
				if err == nil {
					outObj := &unstructured.Unstructured{}
					outObj.SetGroupVersionKind(out.OutputStore.TypedReference.GroupVersionKind())
					outObj.SetName(out.OutputStore.TypedReference.Name)
					toPath := oper.ToFieldPath
					if len(oper.ToDataPath) != 0 {
						toPath = toPath + "(" + oper.ToDataPath + ")"
					}
					uds = append(uds, makeUnsatisfiedDependency(outObj, newS, []string{toPath}, reason))
				}
				allConditionsReady = false
				break
			}
		}
		if allConditionsReady {
			outputMap[out.Name] = out
		}
	}
	return uds, outputMap
}

func (r *components) handleDataInput(ctx context.Context, ins []v1alpha2.DataInput, dag *dag, obj, ac *unstructured.Unstructured) ([]v1alpha2.UnstaifiedDependency, error) {
	uds := make([]v1alpha2.UnstaifiedDependency, 0)
	for _, in := range ins {
		if !reflect.DeepEqual(in.ValueFrom, v1alpha2.DataInputValueFrom{}) && len(strings.TrimSpace(in.ValueFrom.DataOutputName)) != 0 {
			dep, err := r.handleDataOutputConds(ctx, in, dag, obj, ac)
			if dep != nil {
				uds = append(uds, *dep)
				return uds, err
			}
			if err != nil {
				return nil, err
			}
		}
		if !reflect.DeepEqual(in.InputStore, v1alpha2.StoreReference{}) {
			dep, err := r.handleDataStoreConds(ctx, in, obj, ac)
			if dep != nil {
				uds = append(uds, *dep)
				return uds, err
			}
			if err != nil {
				return nil, err
			}
		}

		if len(in.Conditions) != 0 {
			dep, err := r.handleDataInputConds(ctx, in, dag, obj, ac)
			if dep != nil {
				uds = append(uds, *dep)
				return uds, err
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return uds, nil
}

func (r *components) handleDataOutputConds(ctx context.Context, in v1alpha2.DataInput, dag *dag, obj, ac *unstructured.Unstructured) (*v1alpha2.UnstaifiedDependency, error) {
	s, ok := dag.Sources[in.ValueFrom.DataOutputName]
	if !ok {
		return nil, errors.Wrapf(ErrDataOutputNotExist, "DataOutputName (%s)", in.ValueFrom.DataOutputName)
	}
	val, ready, reason, err := r.getDataInput(ctx, s, ac, false)
	if err != nil {
		return nil, errors.Wrap(err, "getDataInput failed")
	}
	if !ready {
		dep := makeUnsatisfiedDependency(obj, s, in.ToFieldPaths, reason)
		return &dep, nil
	}
	err = fillDataInputValue(obj, in.ToFieldPaths, val, in.StrategyMergeKeys)
	if err != nil {
		return nil, errors.Wrap(err, "fillDataInputValue failed")
	}
	return nil, nil
}

func (r *components) handleDataStoreConds(ctx context.Context, in v1alpha2.DataInput, obj, ac *unstructured.Unstructured) (*v1alpha2.UnstaifiedDependency, error) {
	for _, oper := range in.InputStore.Operations {
		s := &dagSource{
			ObjectRef: &corev1.ObjectReference{
				APIVersion: in.InputStore.APIVersion,
				Kind:       in.InputStore.Kind,
				Name:       in.InputStore.Name,
				// according current implementation, outputRef use the namespace of workload which is set with the namespace of ac. So it's ok to use ac.GetNamespace() here.
				// obj.GetNamespace() may be empty when obj has not been created.
				Namespace: ac.GetNamespace(),
				FieldPath: oper.ValueFrom.FieldPath,
			},
			Conditions: oper.Conditions,
		}
		_, ready, reason, err := r.getDataInput(ctx, s, ac, false)
		if err != nil {
			return nil, errors.Wrap(err, "getDataInput failed")
		}
		if !ready {
			toPath := oper.ToFieldPath
			if len(oper.ToDataPath) != 0 {
				toPath = toPath + "(" + oper.ToDataPath + ")"
			}
			dep := makeUnsatisfiedDependency(obj, s, []string{toPath}, reason)
			return &dep, nil
		}
	}
	return nil, nil
}
func (r *components) handleDataInputConds(ctx context.Context, in v1alpha2.DataInput, dag *dag, obj, ac *unstructured.Unstructured) (*v1alpha2.UnstaifiedDependency, error) {
	_, ok := dag.Sources[in.ValueFrom.DataOutputName]
	if !ok {
		return nil, errors.Wrapf(ErrDataOutputNotExist, "DataOutputName (%s)", in.ValueFrom.DataOutputName)
	}
	s := &dagSource{
		ObjectRef: &corev1.ObjectReference{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			// according current implementation, outputRef use the namespace of workload which is set with the namespace of ac. So it's ok to use ac.GetNamespace() here.
			// obj.GetNamespace() may be empty when obj has not been created.
			Namespace: ac.GetNamespace(),
		},
		Conditions: in.Conditions,
	}
	_, ready, reason, err := r.getDataInput(ctx, s, ac, true)
	if err != nil {
		return nil, errors.Wrap(err, "getDataInput failed")
	}
	if !ready {
		dep := makeUnsatisfiedDependency(obj, dag.Sources[in.ValueFrom.DataOutputName], in.ToFieldPaths, "DataInputs Conditions: "+reason)
		return &dep, nil
	}
	return nil, nil
}

func (r *components) getDataInput(ctx context.Context, s *dagSource, ac *unstructured.Unstructured, ignoreNotFound bool) (interface{}, bool, string, error) {
	obj := s.ObjectRef
	key := types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      obj.Name,
	}
	// If obj.FieldPath is empty and the length of dagSource's Conditions is 0, return true
	if len(obj.FieldPath) == 0 && len(s.Conditions) == 0 {
		return nil, true, "", nil
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(obj.GroupVersionKind())
	err := r.client.Get(ctx, key, u)
	if err != nil {
		if resource.IgnoreNotFound(err) == nil && ignoreNotFound {
			return nil, true, "", nil
		}
		reason := fmt.Sprintf("failed to get object (%s)", key.String())
		return nil, false, reason, errors.Wrap(resource.IgnoreNotFound(err), reason)
	}
	paved := fieldpath.Pave(u.UnstructuredContent())
	pavedAC := fieldpath.Pave(ac.UnstructuredContent())
	rawval, err := paved.GetValue(obj.FieldPath)
	if err != nil {
		if fieldpath.IsNotFound(err) {
			return "", false, fmt.Sprintf("%s not found in object", obj.FieldPath), nil
		}
		err = fmt.Errorf("failed to get field value (%s) in object (%s): %w", obj.FieldPath, key.String(), err)
		return nil, false, err.Error(), err
	}

	var ok bool
	var reason string
	switch val := rawval.(type) {
	case string:
		// For string input we will:
		// - check its value not empty if no condition is given.
		// - check its value against conditions if no field path is specified.
		ok, reason = matchValue(s.Conditions, val, paved, pavedAC)
	default:
		ok, reason = checkConditions(s.Conditions, paved, nil, pavedAC)
	}
	if !ok {
		return nil, false, reason, nil
	}

	return rawval, true, "", nil
}

func isControlledByApp(ac *v1alpha2.ApplicationConfiguration) bool {
	for _, owner := range ac.GetOwnerReferences() {
		if owner.APIVersion == v1alpha2.SchemeGroupVersion.String() && owner.Kind == v1alpha2.ApplicationKind &&
			owner.Controller != nil && *owner.Controller {
			return true
		}
	}
	return false
}

func matchValue(conds []v1alpha2.ConditionRequirement, val string, paved, ac *fieldpath.Paved) (bool, string) {
	// If no condition is specified, it is by default to check value not empty.
	if len(conds) == 0 {
		if val == "" {
			return false, "value should not be empty"
		}
		return true, ""
	}

	return checkConditions(conds, paved, &val, ac)
}

func getCheckVal(m v1alpha2.ConditionRequirement, paved *fieldpath.Paved, val *string) (string, error) {
	var checkVal string
	switch {
	case m.FieldPath != "":
		return paved.GetString(m.FieldPath)
	case val != nil:
		checkVal = *val
	default:
		return "", errors.New("FieldPath not specified")
	}
	return checkVal, nil
}

func getExpectVal(m v1alpha2.ConditionRequirement, ac *fieldpath.Paved) (string, error) {
	if m.Value != "" {
		return m.Value, nil
	}
	if m.ValueFrom.FieldPath == "" || ac == nil {
		return "", nil
	}
	var err error
	value, err := ac.GetString(m.ValueFrom.FieldPath)
	if err != nil {
		return "", fmt.Errorf("get valueFrom.fieldPath fail: %w", err)
	}
	return value, nil
}

func checkConditions(conds []v1alpha2.ConditionRequirement, paved *fieldpath.Paved, val *string, ac *fieldpath.Paved) (bool, string) {
	for _, m := range conds {
		checkVal, err := getCheckVal(m, paved, val)
		if err != nil {
			return false, fmt.Sprintf("can't get value to check %v", err)
		}
		m.Value, err = getExpectVal(m, ac)
		if err != nil {
			return false, err.Error()
		}

		switch m.Operator {
		case v1alpha2.ConditionEqual:
			if m.Value != checkVal {
				return false, fmt.Sprintf("got(%v) expected to be %v", checkVal, m.Value)
			}
		case v1alpha2.ConditionNotEqual:
			if m.Value == checkVal {
				return false, fmt.Sprintf("got(%v) expected not to be %v", checkVal, m.Value)
			}
		case v1alpha2.ConditionNotEmpty:
			if checkVal == "" {
				return false, "value should not be empty"
			}
		}
	}
	return true, ""
}

// GetTraitName return trait name
func getTraitName(ac *v1alpha2.ApplicationConfiguration, componentName string,
	ct *v1alpha2.ComponentTrait, t *unstructured.Unstructured, traitDef *v1alpha2.TraitDefinition) string {
	var traitName, apiVersion, kind string
	// we forbid the trait name in the template if the applicationConfiguration is generated by application
	if len(t.GetName()) > 0 && !isControlledByApp(ac) {
		return t.GetName()
	}

	apiVersion = t.GetAPIVersion()
	kind = t.GetKind()

	traitType := traitDef.Name
	if strings.Contains(traitType, ".") {
		traitType = strings.Split(traitType, ".")[0]
	}

	for _, w := range ac.Status.Workloads {
		if w.ComponentName != componentName {
			continue
		}
		for _, trait := range w.Traits {
			if trait.Reference.APIVersion == apiVersion && trait.Reference.Kind == kind {
				traitName = trait.Reference.Name
			}
		}
	}

	if len(traitName) == 0 {
		traitName = util.GenTraitName(componentName, ct.DeepCopy(), traitType)
	}

	return traitName
}

// getExistingWorkload tries to retrieve the currently running workload
func (r *components) getExistingWorkload(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, c *v1alpha2.Component, w *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var workloadName string
	existingWorkload := &unstructured.Unstructured{}
	for _, component := range ac.Status.Workloads {
		if component.ComponentName != c.GetName() {
			continue
		}
		workloadName = component.Reference.Name
	}
	if workloadName != "" {
		objectKey := client.ObjectKey{Namespace: ac.GetNamespace(), Name: workloadName}
		existingWorkload.SetAPIVersion(w.GetAPIVersion())
		existingWorkload.SetKind(w.GetKind())
		err := r.client.Get(ctx, objectKey, existingWorkload)
		if err != nil {
			return nil, client.IgnoreNotFound(err)
		}
	}
	return existingWorkload, nil
}

// discoverHelmModuleWorkload will get the workload created by flux/helm-controller
func discoverHelmModuleWorkload(ctx context.Context, c client.Reader, comp *v1alpha2.Component, ns string) (*unstructured.Unstructured, error) {
	if comp == nil || comp.Spec.Helm == nil {
		return nil, errors.New("the component has no valid helm module")
	}

	rls, err := util.RawExtension2Unstructured(&comp.Spec.Helm.Release)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get helm release from component")
	}
	rlsName := rls.GetName()

	chartName, ok, err := unstructured.NestedString(rls.Object, helmapi.HelmChartNamePath...)
	if err != nil || !ok {
		return nil, errors.New("cannot get helm chart name")
	}

	// qualifiedFullName is used as the name of target workload.
	// It strictly follows the convention that Helm generate default full name as below:
	// > We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
	// > If release name contains chart name it will be used as a full name.
	qualifiedWorkloadName := rlsName
	if !strings.Contains(rlsName, chartName) {
		qualifiedWorkloadName = fmt.Sprintf("%s-%s", rlsName, chartName)
		if len(qualifiedWorkloadName) > 63 {
			qualifiedWorkloadName = strings.TrimSuffix(qualifiedWorkloadName[:63], "-")
		}
	}

	wl, err := util.RawExtension2Unstructured(&comp.Spec.Workload)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get workload from component")
	}

	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: qualifiedWorkloadName}, wl); err != nil {
		return nil, err
	}

	// check it's created by helm and match the release info
	annots := wl.GetAnnotations()
	labels := wl.GetLabels()
	if annots == nil || labels == nil ||
		annots["meta.helm.sh/release-name"] != rlsName ||
		annots["meta.helm.sh/release-namespace"] != ns ||
		labels["app.kubernetes.io/managed-by"] != "Helm" {
		err := fmt.Errorf("the workload is found but not match with helm info(meta.helm.sh/release-name: %s, meta.helm.sh/namespace: %s, app.kubernetes.io/managed-by: Helm)",
			rlsName, ns)
		klog.ErrorS(err, "Found a name-matched workload but not managed by Helm", "name", qualifiedWorkloadName,
			"annotations", annots, "labels", labels)
		return nil, err
	}

	return wl, nil
}
