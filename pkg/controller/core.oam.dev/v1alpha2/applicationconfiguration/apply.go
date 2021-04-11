/*
Copyright 2021 The Crossplane Authors.

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
	"strings"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// Reconcile error strings.
const (
	errFmtApplyWorkload            = "cannot apply workload %q"
	errFmtSetWorkloadRef           = "cannot set trait %q reference to %q"
	errFmtSetScopeWorkloadRef      = "cannot set scope %q reference to %q"
	errFmtGetTraitDefinition       = "cannot find trait definition %q %q %q"
	errFmtGetScopeDefinition       = "cannot find scope definition %q %q %q"
	errFmtGetScopeWorkloadRef      = "cannot find scope workloadRef %q %q %q with workloadRefsPath %q"
	errFmtGetScopeWorkloadRefsPath = "cannot get workloadRefsPath for scope to be dereferenced %q %q %q"
	errFmtApplyTrait               = "cannot apply trait %q %q %q"
	errFmtApplyScope               = "cannot apply scope %q %q %q"

	workloadScopeFinalizer      = "scope.finalizer.core.oam.dev"
	dot                    byte = '.'
	slash                  byte = '/'
	dQuotes                byte = '"'
)

var (
	// ErrInvaildOperationType describes the error that Operator of DataOperation is not in defined DataOperator
	ErrInvaildOperationType = errors.New("invaild type in operation")
	// ErrInvaildOperationValueAndValueFrom describes the error that both value and valueFrom in DataOperation are empty
	ErrInvaildOperationValueAndValueFrom = errors.New("invaild value and valueFrom in operation: both are empty")
)

// A WorkloadApplicator creates or updates or finalizes workloads and their traits.
type WorkloadApplicator interface {
	// Apply a workload and its traits.
	Apply(ctx context.Context, status []v1alpha2.WorkloadStatus, w []Workload, ao ...apply.ApplyOption) error

	// Finalize implements pre-delete hooks on workloads
	Finalize(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) error
}

// A WorkloadApplyFns creates or updates or finalizes workloads and their traits.
type WorkloadApplyFns struct {
	ApplyFn    func(ctx context.Context, status []v1alpha2.WorkloadStatus, w []Workload, ao ...apply.ApplyOption) error
	FinalizeFn func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) error
}

// Apply a workload and its traits. It employes the same mechanism as `kubectl apply`, that is, for each resource being applied,
// computing a three-way diff merge in client side based on its current state, modified stated and last-applied-state which is
// tracked through an specific annotaion. If the resource doesn't exist before, Apply will create it.
func (fn WorkloadApplyFns) Apply(ctx context.Context, status []v1alpha2.WorkloadStatus, w []Workload, ao ...apply.ApplyOption) error {
	return fn.ApplyFn(ctx, status, w, ao...)
}

// Finalize workloads and its traits/scopes.
func (fn WorkloadApplyFns) Finalize(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) error {
	return fn.FinalizeFn(ctx, ac)
}

type workloads struct {
	applicator apply.Applicator
	rawClient  client.Client
	dm         discoverymapper.DiscoveryMapper
}

func (a *workloads) Apply(ctx context.Context, status []v1alpha2.WorkloadStatus, w []Workload,
	ao ...apply.ApplyOption) error {
	// they are all in the same namespace
	var namespace = w[0].Workload.GetNamespace()
	for _, wl := range w {
		if !wl.HasDep {
			if wl.SkipApply {
				klog.InfoS("skip apply a workload due to rollout", "component name", wl.ComponentName, "component revision",
					wl.ComponentRevisionName)
			} else {
				// Apply the DataInputs to this workload
				if err := a.ApplyInputRef(ctx, wl.Workload, wl.DataInputs, namespace, ao...); err != nil {
					return err
				}
				if err := a.applicator.Apply(ctx, wl.Workload, ao...); err != nil {
					if !errors.Is(err, &GenerationUnchanged{}) {
						// GenerationUnchanged only aborts applying current workload
						// but not blocks the whole reconciliation through returning an error
						return errors.Wrapf(err, errFmtApplyWorkload, wl.Workload.GetName())
					}
				}
			}

		}
		// Apply the ready DatatOutputs of this workload
		if err := a.ApplyOutputRef(ctx, wl.Workload, wl.DataOutputs, namespace, ao...); err != nil {
			return err
		}
		for _, trait := range wl.Traits {
			if !trait.HasDep {
				if err := a.ApplyInputRef(ctx, &trait.Object, trait.DataInputs, namespace, ao...); err != nil {
					return err
				}
				t := trait.Object
				if err := a.applicator.Apply(ctx, &trait.Object, ao...); err != nil {
					if !errors.Is(err, &GenerationUnchanged{}) {
						// GenerationUnchanged only aborts applying current trait
						// but not blocks the whole reconciliation through returning an error
						return errors.Wrapf(err, errFmtApplyTrait, t.GetAPIVersion(), t.GetKind(), t.GetName())
					}
				}
			}
			if err := a.ApplyOutputRef(ctx, &trait.Object, trait.DataOutputs, namespace, ao...); err != nil {
				return err
			}
		}
		workloadRef := runtimev1alpha1.TypedReference{
			APIVersion: wl.Workload.GetAPIVersion(),
			Kind:       wl.Workload.GetKind(),
			Name:       wl.Workload.GetName(),
		}
		for _, s := range wl.Scopes {
			if err := a.applyScope(ctx, wl, s, workloadRef); err != nil {
				return err
			}
		}
	}
	return a.dereferenceScope(ctx, namespace, status, w)
}

func (a *workloads) ApplyOutputRef(ctx context.Context, w *unstructured.Unstructured, outputs map[string]v1alpha2.DataOutput, namespace string, ao ...apply.ApplyOption) error {
	for _, output := range outputs {
		if reflect.DeepEqual(output, v1alpha2.DataOutput{}) || reflect.DeepEqual(output.OutputStore, v1alpha2.StoreReference{}) {
			continue
		}
		// Get the running workload
		runningW := &unstructured.Unstructured{}
		runningW.SetAPIVersion(w.GetAPIVersion())
		runningW.SetKind(w.GetKind())
		key := types.NamespacedName{
			Namespace: w.GetNamespace(),
			Name:      w.GetName(),
		}
		if err := a.rawClient.Get(ctx, key, runningW); err != nil {
			return err
		}
		// Get the outputRef object
		ref := &unstructured.Unstructured{}
		ref.SetAPIVersion(output.OutputStore.APIVersion)
		ref.SetKind(output.OutputStore.Kind)
		key = types.NamespacedName{
			Namespace: namespace,
			Name:      output.OutputStore.Name,
		}
		if err := a.rawClient.Get(ctx, key, ref); err != nil {
			if resource.IgnoreNotFound(err) != nil {
				return err
			}
			// Create the outputRef object if it doesn't exist
			ref.SetNamespace(namespace)
			ref.SetName(output.OutputStore.Name)
			ref.SetOwnerReferences(runningW.GetOwnerReferences())
			if err := a.applicator.Apply(ctx, ref, ao...); err != nil {
				return err
			}
			if err = a.rawClient.Get(ctx, key, ref); err != nil {
				return err
			}
		}
		for _, oper := range output.OutputStore.Operations {
			if err := operationProcess(ref, runningW, oper); err != nil {
				return err
			}
		}
		if err := a.applicator.Apply(ctx, ref, ao...); err != nil {
			return err
		}
	}
	return nil
}
func (a *workloads) ApplyInputRef(ctx context.Context, w *unstructured.Unstructured, inputs []v1alpha2.DataInput, namespace string, ao ...apply.ApplyOption) error {
	for _, input := range inputs {
		if reflect.DeepEqual(input, v1alpha2.DataInput{}) || reflect.DeepEqual(input.InputStore, v1alpha2.StoreReference{}) {
			continue
		}
		ref := &unstructured.Unstructured{}
		ref.SetAPIVersion(input.InputStore.APIVersion)
		ref.SetKind(input.InputStore.Kind)
		key := types.NamespacedName{
			Namespace: namespace,
			Name:      input.InputStore.Name,
		}
		if err := a.rawClient.Get(ctx, key, ref); err != nil {
			return err
		}
		for _, oper := range input.InputStore.Operations {
			if err := operationProcess(w, ref, oper); err != nil {
				return err
			}
		}
	}
	return nil
}
func operationProcess(inputObj *unstructured.Unstructured, outputObj *unstructured.Unstructured, oper v1alpha2.DataOperation) error {
	switch oper.Type {
	case "jsonPatch":
		jsonBytes, err := json.Marshal(inputObj)
		if err != nil {
			return err
		}
		targetJSON := []byte(gjson.GetBytes(jsonBytes, oper.ToFieldPath).String())
		value := ""
		switch {
		case len(oper.Value) != 0:
			value = oper.Value
		case len(oper.ValueFrom.FieldPath) != 0:
			v, err := getValueFromPath(outputObj, oper.ValueFrom.FieldPath)
			if err != nil {
				return err
			}
			vJSON, err := json.Marshal(v)
			if err != nil {
				return err
			}
			value = string(vJSON)
		default:
			return ErrInvaildOperationValueAndValueFrom
		}
		targetJSON, err = jsonOperation(targetJSON, oper.Operator, oper.ToDataPath, value, oper.ToDataPath)
		if err != nil {
			return err
		}
		jsonBytes, err = jsonOperation(jsonBytes, v1alpha2.ReplaceOperator, oper.ToFieldPath, string(targetJSON), oper.ToDataPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(jsonBytes, inputObj); err != nil {
			return errors.Wrap(err, errUnmarshalWorkload)
		}
		return nil
	default:
		return ErrInvaildOperationType
	}
}

func (a *workloads) Finalize(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) error {
	var namespace = ac.GetNamespace()

	if meta.FinalizerExists(&ac.ObjectMeta, workloadScopeFinalizer) {
		if err := a.dereferenceAllScopes(ctx, namespace, ac.Status.Workloads); err != nil {
			return err
		}
		meta.RemoveFinalizer(&ac.ObjectMeta, workloadScopeFinalizer)
	}

	// add finalizer logic here
	return nil
}

func (a *workloads) dereferenceScope(ctx context.Context, namespace string, status []v1alpha2.WorkloadStatus, w []Workload) error {
	for _, st := range status {
		toBeDeferenced := st.Scopes
		for _, wl := range w {
			if (st.Reference.APIVersion == wl.Workload.GetAPIVersion()) &&
				(st.Reference.Kind == wl.Workload.GetKind()) &&
				(st.Reference.Name == wl.Workload.GetName()) {
				toBeDeferenced = findDereferencedScopes(st.Scopes, wl.Scopes)
			}
		}

		for _, s := range toBeDeferenced {
			if err := a.applyScopeRemoval(ctx, namespace, st.Reference, s); err != nil {
				return err
			}
		}
	}

	return nil
}

// dereferenceAllScope dereferences workloads owned by the appConfig being deleted from the scopes they belong to.
func (a *workloads) dereferenceAllScopes(ctx context.Context, namespace string, status []v1alpha2.WorkloadStatus) error {
	for _, st := range status {
		for _, sc := range st.Scopes {
			if err := a.applyScopeRemoval(ctx, namespace, st.Reference, sc); err != nil {
				return err
			}
		}
	}

	return nil
}

func findDereferencedScopes(statusScopes []v1alpha2.WorkloadScope, scopes []unstructured.Unstructured) []v1alpha2.WorkloadScope {
	toBeDeferenced := []v1alpha2.WorkloadScope{}
	for _, ss := range statusScopes {
		found := false
		for _, s := range scopes {
			if (s.GetAPIVersion() == ss.Reference.APIVersion) &&
				(s.GetKind() == ss.Reference.Kind) &&
				(s.GetName() == ss.Reference.Name) {
				found = true
				break
			}
		}

		if !found {
			toBeDeferenced = append(toBeDeferenced, ss)
		}
	}

	return toBeDeferenced
}

func (a *workloads) applyScope(ctx context.Context, wl Workload, s unstructured.Unstructured, workloadRef runtimev1alpha1.TypedReference) error {
	// get ScopeDefinition
	scopeDefinition, err := util.FetchScopeDefinition(ctx, a.rawClient, a.dm, &s)
	if err != nil {
		return errors.Wrapf(err, errFmtGetScopeDefinition, s.GetAPIVersion(), s.GetKind(), s.GetName())
	}
	// checkout whether scope asks for workloadRef
	workloadRefsPath := scopeDefinition.Spec.WorkloadRefsPath
	if len(workloadRefsPath) == 0 {
		// this scope does not ask for workloadRefs
		return nil
	}

	var refs []interface{}
	if value, err := fieldpath.Pave(s.UnstructuredContent()).GetValue(workloadRefsPath); err == nil {
		refs = value.([]interface{})

		for _, item := range refs {
			ref := item.(map[string]interface{})
			if (workloadRef.APIVersion == ref["apiVersion"]) &&
				(workloadRef.Kind == ref["kind"]) &&
				(workloadRef.Name == ref["name"]) {
				// workloadRef is already present, so no need to add it.
				return nil
			}
		}
	} else {
		return errors.Wrapf(err, errFmtGetScopeWorkloadRef, s.GetAPIVersion(), s.GetKind(), s.GetName(), workloadRefsPath)
	}

	refs = append(refs, workloadRef)
	if err := fieldpath.Pave(s.UnstructuredContent()).SetValue(workloadRefsPath, refs); err != nil {
		return errors.Wrapf(err, errFmtSetScopeWorkloadRef, s.GetName(), wl.Workload.GetName())
	}

	if err := a.rawClient.Update(ctx, &s); err != nil {
		return errors.Wrapf(err, errFmtApplyScope, s.GetAPIVersion(), s.GetKind(), s.GetName())
	}

	return nil
}

// applyScopeRemoval remove the workload reference from the scope's reference list.
// If the scope or scope definition is not found(deleted), it's still regarded as remove successfully.
func (a *workloads) applyScopeRemoval(ctx context.Context, namespace string, wr runtimev1alpha1.TypedReference, s v1alpha2.WorkloadScope) error {
	scopeObject := unstructured.Unstructured{}
	scopeObject.SetAPIVersion(s.Reference.APIVersion)
	scopeObject.SetKind(s.Reference.Kind)
	scopeObjectRef := types.NamespacedName{Namespace: namespace, Name: s.Reference.Name}
	if err := a.rawClient.Get(ctx, scopeObjectRef, &scopeObject); err != nil {
		// if the scope is already deleted
		// treat it as removal done to avoid blocking AppConfig finalizer
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, errFmtApplyScope, s.Reference.APIVersion, s.Reference.Kind, s.Reference.Name)
	}

	scopeDefinition, err := util.FetchScopeDefinition(ctx, a.rawClient, a.dm, &scopeObject)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// if the scope definition is deleted
			// treat it as removal done to avoid blocking AppConfig finalizer
			return nil
		}
		return errors.Wrapf(err, errFmtGetScopeDefinition, scopeObject.GetAPIVersion(), scopeObject.GetKind(), scopeObject.GetName())
	}

	workloadRefsPath := scopeDefinition.Spec.WorkloadRefsPath
	if len(workloadRefsPath) == 0 {
		// Scopes to be dereferenced MUST have workloadRefsPath
		return errors.Errorf(errFmtGetScopeWorkloadRefsPath, scopeObject.GetAPIVersion(), scopeObject.GetKind(), scopeObject.GetName())
	}

	if value, err := fieldpath.Pave(scopeObject.UnstructuredContent()).GetValue(workloadRefsPath); err == nil {
		refs := value.([]interface{})

		workloadRefIndex := -1
		for i, item := range refs {
			ref := item.(map[string]interface{})
			if (wr.APIVersion == ref["apiVersion"]) &&
				(wr.Kind == ref["kind"]) &&
				(wr.Name == ref["name"]) {
				workloadRefIndex = i
				break
			}
		}

		if workloadRefIndex >= 0 {
			// Remove the element at index i.
			refs[workloadRefIndex] = refs[len(refs)-1]
			refs = refs[:len(refs)-1]

			if err := fieldpath.Pave(scopeObject.UnstructuredContent()).SetValue(workloadRefsPath, refs); err != nil {
				return errors.Wrapf(err, errFmtSetScopeWorkloadRef, s.Reference.Name, wr.Name)
			}

			if err := a.rawClient.Update(ctx, &scopeObject); err != nil {
				return errors.Wrapf(err, errFmtApplyScope, s.Reference.APIVersion, s.Reference.Kind, s.Reference.Name)
			}
		}
	} else {
		return errors.Wrapf(err, errFmtGetScopeWorkloadRef,
			scopeObject.GetAPIVersion(), scopeObject.GetKind(), scopeObject.GetName(), workloadRefsPath)
	}
	return nil
}

func getValueFromPath(w *unstructured.Unstructured, path string) (interface{}, error) {
	paved := fieldpath.Pave(w.UnstructuredContent())
	rawval, err := paved.GetValue(path)
	if err != nil {
		if fieldpath.IsNotFound(err) {
			return nil, fmt.Errorf("%s not found in object", path)
		}
		err = fmt.Errorf("failed to get field value (%s) in object (%s:%s): %w", path, w.GetNamespace(), w.GetName(), err)
		return nil, err
	}
	return rawval, nil
}

func jsonOperation(jsonBytes []byte, op v1alpha2.DataOperator, path, value, toDataPath string) ([]byte, error) {
	if len(jsonBytes) == 0 || len(path) == 0 {
		return []byte(value), nil
	}
	patchJSON := []byte(`[{"op": "` + string(op) + `", "path": "`)
	// \. is used to escape dot, @@@DOTDOTDOT@@@ is used to avoid replacement of dot in following operation
	path = strings.ReplaceAll(path, `\.`, `@@@DOTDOTDOT@@@`)
	path = strings.ReplaceAll(path, string(dot), string(slash))
	path = strings.ReplaceAll(path, `@@@DOTDOTDOT@@@`, `.`)
	if path[0] != slash {
		patchJSON = append(patchJSON, slash)
	}
	value = strings.ReplaceAll(strings.ReplaceAll(value, `\"`, `"`), `\\`, `\`)
	if len(value) > 1 && value[0] == dQuotes && value[len(value)-1] == dQuotes {
		value = string(dQuotes) + strings.ReplaceAll(strings.ReplaceAll(value[1:len(value)-1], `\`, `\\`), `"`, `\"`) + string(dQuotes)
	} else if len(toDataPath) > 0 {
		value = string(dQuotes) + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + string(dQuotes)
	}
	patchJSON = append(patchJSON, []byte(path+`", "value": `+value+`}]`)...)
	patch, err := jsonpatch.DecodePatch(patchJSON)
	if err != nil {
		return nil, err
	}
	return patch.Apply(jsonBytes)
}
