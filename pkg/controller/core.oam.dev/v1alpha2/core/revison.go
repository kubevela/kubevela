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

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/pkg/errors"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// GenerateDefinitionRevision will generate a definition revision the generated revision
// will be compare with the last revision to see if there's any difference.
func GenerateDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object) (*v1beta1.DefinitionRevision, bool, error) {
	isNamedRev, defRevNamespacedName, err := isNamedRevision(def)
	if err != nil {
		return nil, false, err
	}
	if isNamedRev {
		return generateNamedDefinitionRevision(ctx, cli, def, defRevNamespacedName)
	}

	defRev, lastRevision, err := GatherRevisionInfo(def)
	if err != nil {
		return defRev, false, err
	}
	isNewRev, err := compareWithLastDefRevisionSpec(ctx, cli, defRev, lastRevision)
	if err != nil {
		return defRev, isNewRev, err
	}
	if isNewRev {
		defRevName, revNum := getDefNextRevision(defRev, lastRevision)
		defRev.Name = defRevName
		defRev.Spec.Revision = revNum
	}
	return defRev, isNewRev, nil
}

func isNamedRevision(def runtime.Object) (bool, types.NamespacedName, error) {
	defMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(def)
	if err != nil {
		return false, types.NamespacedName{}, err
	}
	unstructuredDef := unstructured.Unstructured{
		Object: defMap,
	}
	revisionName := unstructuredDef.GetAnnotations()[oam.AnnotationDefinitionRevisionName]
	if len(revisionName) == 0 {
		return false, types.NamespacedName{}, nil
	}
	defNs := unstructuredDef.GetNamespace()
	defName := unstructuredDef.GetName()
	defRevName := ConstructDefinitionRevisionName(defName, revisionName)
	return true, types.NamespacedName{Name: defRevName, Namespace: defNs}, nil
}

func generateNamedDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, defRevNamespacedName types.NamespacedName) (*v1beta1.DefinitionRevision, bool, error) {
	oldDefRev := new(v1beta1.DefinitionRevision)

	// definitionRevision is immutable, if the requested definitionRevision already exists, return directly.
	err := cli.Get(ctx, defRevNamespacedName, oldDefRev)
	if err == nil {
		return oldDefRev, false, nil
	}

	if apierrors.IsNotFound(err) {
		newDefRev, lastRevision, err := GatherRevisionInfo(def)
		if err != nil {
			return newDefRev, false, err
		}
		_, revNum := getDefNextRevision(newDefRev, lastRevision)
		newDefRev.Name = defRevNamespacedName.Name
		newDefRev.Spec.Revision = revNum
		return newDefRev, true, nil
	}
	return nil, false, err
}

// GatherRevisionInfo gather revision information from definition
func GatherRevisionInfo(def runtime.Object) (*v1beta1.DefinitionRevision, *common.Revision, error) {
	defRev := &v1beta1.DefinitionRevision{}
	var LastRevision *common.Revision
	switch definition := def.(type) {
	case *v1beta1.ComponentDefinition:
		copiedCompDef := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.ComponentType
		defRev.Spec.ComponentDefinition = *copiedCompDef
		LastRevision = copiedCompDef.Status.LatestRevision
	case *v1beta1.TraitDefinition:
		copiedTraitDef := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.TraitType
		defRev.Spec.TraitDefinition = *copiedTraitDef
		LastRevision = copiedTraitDef.Status.LatestRevision
	case *v1beta1.PolicyDefinition:
		defCopy := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.PolicyType
		defRev.Spec.PolicyDefinition = *defCopy
		LastRevision = defCopy.Status.LatestRevision
	case *v1beta1.WorkflowStepDefinition:
		defCopy := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.WorkflowStepType
		defRev.Spec.WorkflowStepDefinition = *defCopy
		LastRevision = defCopy.Status.LatestRevision
	default:
		return nil, nil, fmt.Errorf("unsupported type %v", definition)
	}

	defHash, err := computeDefinitionRevisionHash(defRev)
	if err != nil {
		return nil, nil, err
	}
	defRev.Spec.RevisionHash = defHash
	return defRev, LastRevision, nil
}

func computeDefinitionRevisionHash(defRev *v1beta1.DefinitionRevision) (string, error) {
	var defHash string
	var err error
	switch defRev.Spec.DefinitionType {
	case common.ComponentType:
		defHash, err = utils.ComputeSpecHash(&defRev.Spec.ComponentDefinition.Spec)
		if err != nil {
			return defHash, err
		}
	case common.TraitType:
		defHash, err = utils.ComputeSpecHash(&defRev.Spec.TraitDefinition.Spec)
		if err != nil {
			return defHash, err
		}
	case common.PolicyType:
		defHash, err = utils.ComputeSpecHash(&defRev.Spec.PolicyDefinition.Spec)
		if err != nil {
			return defHash, err
		}
	case common.WorkflowStepType:
		defHash, err = utils.ComputeSpecHash(&defRev.Spec.WorkflowStepDefinition.Spec)
		if err != nil {
			return defHash, err
		}
	}
	return defHash, nil
}

func compareWithLastDefRevisionSpec(ctx context.Context, cli client.Client,
	newDefRev *v1beta1.DefinitionRevision, lastRevision *common.Revision) (bool, error) {
	if lastRevision == nil {
		return true, nil
	}
	if lastRevision.RevisionHash != newDefRev.Spec.RevisionHash {
		return true, nil
	}

	// check if the DefinitionRevision is deep equal in Spec level
	// get the last revision from K8s and double check
	defRev := &v1beta1.DefinitionRevision{}
	var namespace string
	switch newDefRev.Spec.DefinitionType {
	case common.ComponentType:
		namespace = newDefRev.Spec.ComponentDefinition.Namespace
	case common.TraitType:
		namespace = newDefRev.Spec.TraitDefinition.Namespace
	case common.PolicyType:
		namespace = newDefRev.Spec.PolicyDefinition.Namespace
	case common.WorkflowStepType:
		namespace = newDefRev.Spec.WorkflowStepDefinition.Namespace
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: lastRevision.Name,
		Namespace: namespace}, defRev); err != nil {
		return false, errors.Wrapf(err, "get the definitionRevision %s", lastRevision.Name)
	}

	if DeepEqualDefRevision(defRev, newDefRev) {
		// No difference on spec, will not create a new revision
		// align the name and resourceVersion
		newDefRev.Name = defRev.Name
		newDefRev.Spec.Revision = defRev.Spec.Revision
		newDefRev.ResourceVersion = defRev.ResourceVersion
		return false, nil
	}
	// if reach here, it's same hash but different spec
	return true, nil
}

// DeepEqualDefRevision deep compare the spec of definitionRevisions
func DeepEqualDefRevision(old, new *v1beta1.DefinitionRevision) bool {
	if !apiequality.Semantic.DeepEqual(old.Spec.ComponentDefinition.Spec, new.Spec.ComponentDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(old.Spec.TraitDefinition.Spec, new.Spec.TraitDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(old.Spec.PolicyDefinition.Spec, new.Spec.PolicyDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(old.Spec.WorkflowStepDefinition.Spec, new.Spec.WorkflowStepDefinition.Spec) {
		return false
	}
	return true
}

func getDefNextRevision(defRev *v1beta1.DefinitionRevision, lastRevision *common.Revision) (string, int64) {
	var nextRevision int64 = 1
	if lastRevision != nil {
		nextRevision = lastRevision.Revision + 1
	}
	var name string
	switch defRev.Spec.DefinitionType {
	case common.ComponentType:
		name = defRev.Spec.ComponentDefinition.Name
	case common.TraitType:
		name = defRev.Spec.TraitDefinition.Name
	case common.PolicyType:
		name = defRev.Spec.PolicyDefinition.Name
	case common.WorkflowStepType:
		name = defRev.Spec.WorkflowStepDefinition.Name
	}
	defRevName := strings.Join([]string{name, fmt.Sprintf("v%d", nextRevision)}, "-")
	return defRevName, nextRevision
}

// ConstructDefinitionRevisionName construct the name of DefinitionRevision.
func ConstructDefinitionRevisionName(definitionName, revision string) string {
	return strings.Join([]string{definitionName, fmt.Sprintf("v%s", revision)}, "-")
}

// CleanUpDefinitionRevision check all definitionRevisions, remove them if the number of them exceed the limit
func CleanUpDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, revisionLimit int) error {
	var listOpts []client.ListOption
	var usingRevision *common.Revision

	switch definition := def.(type) {
	case *v1beta1.ComponentDefinition:
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelComponentDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision
	case *v1beta1.TraitDefinition:
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelTraitDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision
	case *v1beta1.PolicyDefinition:
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelPolicyDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision
	case *v1beta1.WorkflowStepDefinition:
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelWorkflowStepDefinitionName: definition.Name}}
		usingRevision = definition.Status.LatestRevision
	}

	if usingRevision == nil {
		return nil
	}

	defRevList := new(v1beta1.DefinitionRevisionList)
	if err := cli.List(ctx, defRevList, listOpts...); err != nil {
		return err
	}
	needKill := len(defRevList.Items) - revisionLimit - 1
	if needKill <= 0 {
		return nil
	}
	klog.InfoS("cleanup old definitionRevision", "needKillNum", needKill)

	sortedRevision := defRevList.Items
	sort.Sort(historiesByRevision(sortedRevision))

	for _, rev := range sortedRevision {
		if needKill <= 0 {
			break
		}
		if rev.Name == usingRevision.Name {
			continue
		}
		if err := cli.Delete(ctx, rev.DeepCopy()); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		needKill--
	}
	return nil
}

type historiesByRevision []v1beta1.DefinitionRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Spec.Revision < h[j].Spec.Revision
}

// ReconcileDefinitionRevision generate the definition revision and update it.
func ReconcileDefinitionRevision(ctx context.Context,
	cli client.Client,
	record event.Recorder,
	definition util.ConditionedObject,
	revisionLimit int,
	updateLatestRevision func(*common.Revision) error,
) (*v1beta1.DefinitionRevision, *ctrl.Result, error) {

	// generate DefinitionRevision from componentDefinition
	defRev, isNewRevision, err := GenerateDefinitionRevision(ctx, cli, definition)
	if err != nil {
		klog.ErrorS(err, "Could not generate DefinitionRevision", "componentDefinition", klog.KObj(definition))
		record.Event(definition, event.Warning("Could not generate DefinitionRevision", err))
		return nil, &ctrl.Result{}, util.PatchCondition(ctx, cli, definition,
			condition.ReconcileError(fmt.Errorf(util.ErrGenerateDefinitionRevision, definition.GetName(), err)))
	}

	if isNewRevision {
		if err := CreateDefinitionRevision(ctx, cli, definition, defRev.DeepCopy()); err != nil {
			klog.ErrorS(err, "Could not create DefinitionRevision")
			record.Event(definition, event.Warning("cannot create DefinitionRevision", err))
			return nil, &ctrl.Result{}, util.PatchCondition(ctx, cli, definition,
				condition.ReconcileError(fmt.Errorf(util.ErrCreateDefinitionRevision, defRev.Name, err)))
		}
		klog.InfoS("Successfully created definitionRevision", "definitionRevision", klog.KObj(defRev))

		if err := updateLatestRevision(&common.Revision{
			Name:         defRev.Name,
			Revision:     defRev.Spec.Revision,
			RevisionHash: defRev.Spec.RevisionHash,
		}); err != nil {
			klog.ErrorS(err, "Could not update Definition Status")
			record.Event(definition, event.Warning("cannot update the definition status", err))
			return nil, &ctrl.Result{}, util.PatchCondition(ctx, cli, definition,
				condition.ReconcileError(fmt.Errorf(util.ErrUpdateComponentDefinition, definition.GetName(), err)))
		}
		klog.InfoS("Successfully updated the status.latestRevision of the definition", "Definition", klog.KRef(definition.GetNamespace(), definition.GetName()),
			"Name", defRev.Name, "Revision", defRev.Spec.Revision, "RevisionHash", defRev.Spec.RevisionHash)
	}

	if err = CleanUpDefinitionRevision(ctx, cli, definition, revisionLimit); err != nil {
		klog.InfoS("Failed to collect garbage", "err", err)
		record.Event(definition, event.Warning("failed to garbage collect DefinitionRevision of type ComponentDefinition", err))
	}
	return defRev, nil, nil
}

// CreateDefinitionRevision create the revision of the definition
func CreateDefinitionRevision(ctx context.Context, cli client.Client, def util.ConditionedObject, defRev *v1beta1.DefinitionRevision) error {
	namespace := def.GetNamespace()
	defRev.SetLabels(def.GetLabels())

	var labelKey string
	switch def.(type) {
	case *v1beta1.ComponentDefinition:
		labelKey = oam.LabelComponentDefinitionName
	case *v1beta1.TraitDefinition:
		labelKey = oam.LabelTraitDefinitionName
	case *v1beta1.PolicyDefinition:
		labelKey = oam.LabelPolicyDefinitionName
	case *v1beta1.WorkflowStepDefinition:
		labelKey = oam.LabelWorkflowStepDefinitionName
	}
	if labelKey != "" {
		defRev.SetLabels(util.MergeMapOverrideWithDst(defRev.Labels, map[string]string{labelKey: def.GetName()}))
	} else {
		defRev.SetLabels(defRev.Labels)
	}

	defRev.SetNamespace(namespace)

	rev := &v1beta1.DefinitionRevision{}
	err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: defRev.Name}, rev)
	if apierrors.IsNotFound(err) {
		err = cli.Create(ctx, defRev)
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}
	return err
}
