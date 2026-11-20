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
	"strconv"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/kubevela/pkg/controller/sharding"
	"github.com/pkg/errors"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacache "github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// GenerateDefinitionRevision will generate a definition revision the generated revision
// will be compare with the last revision to see if there's any difference. It reports true
// when the definition's status must point at the returned revision, which is created where it
// does not exist yet.
func GenerateDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object) (*v1beta1.DefinitionRevision, bool, error) {
	isSpecVersion, defRevNamespacedName, err := isSpecVersionRevision(def)
	if err != nil {
		return nil, false, err
	}
	if isSpecVersion {
		return generateDefinitionRevision(ctx, cli, def, defRevNamespacedName)
	}

	isNamedRev, defRevNamespacedName, err := isNameAnnotationRevision(def)
	if err != nil {
		return nil, false, err
	}
	if isNamedRev {
		return generateDefinitionRevision(ctx, cli, def, defRevNamespacedName)
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
		accessor, err := meta.Accessor(def)
		if err != nil {
			return defRev, false, err
		}
		history, err := readRevisionHistory(ctx, cli, accessor.GetNamespace(), defRev.Spec.DefinitionType, accessor.GetName())
		if err != nil {
			return defRev, false, err
		}
		revNum := history.next(lastRevision)
		reuse := func(rev *v1beta1.DefinitionRevision) (*v1beta1.DefinitionRevision, bool, error) {
			defRev.Name = rev.Name
			defRev.Spec.Revision = rev.Spec.Revision
			defRev.ResourceVersion = rev.ResourceVersion
			return defRev, true, nil
		}
		if lastRevision == nil {
			// A definition with no revision of its own yet: new, created again after a delete,
			// or read before its status recorded the revision it just made. It takes back the
			// number of content the name had, existing or recorded, so pins on it keep working.
			if rev := history.holding(defRev); rev != nil {
				return reuse(rev)
			}
			if n := history.hashedNumber(defRev.Spec.RevisionHash); n > 0 {
				defRev.Name = ConstructDefinitionRevisionName(accessor.GetName(), strconv.FormatInt(n, 10))
				defRev.Spec.Revision = n
				return defRev, true, nil
			}
		} else if newest := history.newest(); newest != nil && newest.Spec.Revision == revNum-1 && DeepEqualDefRevision(newest, defRev) {
			// Read before its status recorded the revision it just made.
			return reuse(newest)
		}
		defRev.Name = ConstructDefinitionRevisionName(accessor.GetName(), strconv.FormatInt(revNum, 10))
		defRev.Spec.Revision = revNum
	}
	return defRev, isNewRev, nil
}

// isNameAnnotationRevision is for Definition Version specified in the
// Definition's "definitionrevision.oam.dev/name" annotation.
func isNameAnnotationRevision(def runtime.Object) (bool, types.NamespacedName, error) {
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

// isSpecVersionRevision is for Definition Version specified in the Definition spec.Version
func isSpecVersionRevision(def runtime.Object) (bool, types.NamespacedName, error) {

	var definitionVersion, definitionNamespace, definitionName string
	switch definition := def.(type) {
	case *v1beta1.ComponentDefinition:
		definitionVersion = definition.Spec.Version
		definitionNamespace = definition.Namespace
		definitionName = definition.Name
	case *v1beta1.TraitDefinition:
		definitionVersion = definition.Spec.Version
		definitionNamespace = definition.Namespace
		definitionName = definition.Name
	case *v1beta1.PolicyDefinition:
		definitionVersion = definition.Spec.Version
		definitionNamespace = definition.Namespace
		definitionName = definition.Name
	case *v1beta1.WorkflowStepDefinition:
		definitionVersion = definition.Spec.Version
		definitionNamespace = definition.Namespace
		definitionName = definition.Name
	case *v1beta1.SourceDefinition:
		definitionVersion = definition.Spec.Version
		definitionNamespace = definition.Namespace
		definitionName = definition.Name
	}

	if definitionVersion == "" {
		return false, types.NamespacedName{}, nil
	}
	semVersion, err := semver.NewVersion(definitionVersion)
	if err != nil {
		return false, types.NamespacedName{}, err
	}

	definitionRevisionName := ConstructDefinitionRevisionName(definitionName, semVersion.String())
	return true, types.NamespacedName{Name: definitionRevisionName, Namespace: definitionNamespace}, nil
}

func generateDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, defRevNamespacedName types.NamespacedName) (*v1beta1.DefinitionRevision, bool, error) {
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
		defRev.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: copiedCompDef.APIVersion,
			Kind:       copiedCompDef.Kind,
			Name:       copiedCompDef.Name,
			UID:        copiedCompDef.UID,
		}}
	case *v1beta1.TraitDefinition:
		copiedTraitDef := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.TraitType
		defRev.Spec.TraitDefinition = *copiedTraitDef
		LastRevision = copiedTraitDef.Status.LatestRevision
		defRev.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: copiedTraitDef.APIVersion,
			Kind:       copiedTraitDef.Kind,
			Name:       copiedTraitDef.Name,
			UID:        copiedTraitDef.UID,
		}}

	case *v1beta1.PolicyDefinition:
		defCopy := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.PolicyType
		defRev.Spec.PolicyDefinition = *defCopy
		LastRevision = defCopy.Status.LatestRevision
		defRev.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: defCopy.APIVersion,
			Kind:       defCopy.Kind,
			Name:       defCopy.Name,
			UID:        defCopy.UID,
		}}

	case *v1beta1.WorkflowStepDefinition:
		defCopy := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.WorkflowStepType
		defRev.Spec.WorkflowStepDefinition = *defCopy
		LastRevision = defCopy.Status.LatestRevision
		defRev.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: defCopy.APIVersion,
			Kind:       defCopy.Kind,
			Name:       defCopy.Name,
			UID:        defCopy.UID,
		}}

	case *v1beta1.SourceDefinition:
		defCopy := definition.DeepCopy()
		defRev.Spec.DefinitionType = common.SourceType
		defRev.Spec.SourceDefinition = *defCopy
		LastRevision = defCopy.Status.LatestRevision
		defRev.ObjectMeta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: defCopy.APIVersion,
			Kind:       defCopy.Kind,
			Name:       defCopy.Name,
			UID:        defCopy.UID,
		}}

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

// computeDefinitionRevisionHash hashes the definition's spec, restrictions aside.
func computeDefinitionRevisionHash(defRev *v1beta1.DefinitionRevision) (string, error) {
	spec := withoutRestrictions(defRev)
	switch defRev.Spec.DefinitionType {
	case common.ComponentType:
		return utils.ComputeSpecHash(&spec.ComponentDefinition.Spec)
	case common.TraitType:
		return utils.ComputeSpecHash(&spec.TraitDefinition.Spec)
	case common.PolicyType:
		return utils.ComputeSpecHash(&spec.PolicyDefinition.Spec)
	case common.WorkflowStepType:
		return utils.ComputeSpecHash(&spec.WorkflowStepDefinition.Spec)
	case common.SourceType:
		return utils.ComputeSpecHash(&spec.SourceDefinition.Spec)
	}
	return "", nil
}

func compareWithLastDefRevisionSpec(ctx context.Context, cli client.Client,
	newDefRev *v1beta1.DefinitionRevision, lastRevision *common.Revision) (bool, error) {
	if lastRevision == nil {
		return true, nil
	}

	// The content decides, not the hash: the hash covers the spec's Go struct, so a release
	// that adds a field changes every definition's hash, content unchanged, and the stored
	// revisions Applications pin would be superseded and collected.
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
	case common.SourceType:
		namespace = newDefRev.Spec.SourceDefinition.Namespace
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: lastRevision.Name,
		Namespace: namespace}, defRev); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
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
	return true, nil
}

// DeepEqualDefRevision deep compare the spec of definitionRevisions, restrictions
// aside.
func DeepEqualDefRevision(old, new *v1beta1.DefinitionRevision) bool {
	oldSpec, newSpec := withoutRestrictions(old), withoutRestrictions(new)
	if !apiequality.Semantic.DeepEqual(oldSpec.ComponentDefinition.Spec, newSpec.ComponentDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(oldSpec.TraitDefinition.Spec, newSpec.TraitDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(oldSpec.PolicyDefinition.Spec, newSpec.PolicyDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(oldSpec.WorkflowStepDefinition.Spec, newSpec.WorkflowStepDefinition.Spec) {
		return false
	}
	if !apiequality.Semantic.DeepEqual(oldSpec.SourceDefinition.Spec, newSpec.SourceDefinition.Spec) {
		return false
	}
	return true
}

// withoutRestrictions copies a revision's embedded definitions with their
// restrictions cleared, leaving the caller's object untouched.
//
// Revisions ignore spec.restrictions, in both the hash and the equality check
// that backs it up: a restriction is read from the live definition, so two
// revisions differing only there behave identically. Counting it would roll every
// builtin on a chart-wide restriction and, at the chart's definitionRevisionLimit
// of 2, collect revisions that Applications pin with `type: webservice@v1`.
func withoutRestrictions(defRev *v1beta1.DefinitionRevision) *v1beta1.DefinitionRevisionSpec {
	spec := defRev.Spec.DeepCopy()
	spec.ComponentDefinition.Spec.Restrictions = nil
	spec.TraitDefinition.Spec.Restrictions = nil
	spec.PolicyDefinition.Spec.Restrictions = nil
	spec.WorkflowStepDefinition.Spec.Restrictions = nil
	spec.SourceDefinition.Spec.Restrictions = nil
	return spec
}

func getDefNextRevision(definitionRevision *v1beta1.DefinitionRevision, lastRevision *common.Revision) (string, int64) {
	var nextRevision int64 = 1
	var definitionRevisionName string
	if lastRevision != nil {
		nextRevision = lastRevision.Revision + 1
	}
	var name string
	switch definitionRevision.Spec.DefinitionType {
	case common.ComponentType:
		name = definitionRevision.Spec.ComponentDefinition.Name
	case common.TraitType:
		name = definitionRevision.Spec.TraitDefinition.Name
	case common.PolicyType:
		name = definitionRevision.Spec.PolicyDefinition.Name
	case common.WorkflowStepType:
		name = definitionRevision.Spec.WorkflowStepDefinition.Name
	case common.SourceType:
		name = definitionRevision.Spec.SourceDefinition.Name
	}

	definitionRevisionName = strings.Join([]string{name, fmt.Sprintf("v%v", nextRevision)}, "-")

	return definitionRevisionName, nextRevision
}

// ConstructDefinitionRevisionName construct the name of DefinitionRevision.
func ConstructDefinitionRevisionName(definitionName, revision string) string {
	return strings.Join([]string{definitionName, fmt.Sprintf("v%s", revision)}, "-")
}

// LiveReader reads from the API server, set at setup. The revision garbage collector reads
// Applications through it when the controller is sharded, since a sharded cache holds only this
// shard's Applications; revision numbering reads through it always (liveReader).
var LiveReader client.Reader

// liveReader is LiveReader, or cli where none is set (tests). Revision numbering must see the
// revision a reconcile just made, which the cache may not hold yet, and reading ConfigMaps
// through the cache would start a cluster-wide informer for them.
func liveReader(cli client.Client) client.Reader {
	if LiveReader != nil {
		return LiveReader
	}
	return cli
}

// CleanUpDefinitionRevision deletes a definition's oldest revisions beyond revisionLimit. The
// revision in use and any revision an Application pins (type@vN) are kept and not counted.
func CleanUpDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, revisionLimit int) error {
	// A negative limit keeps no extra revisions, as zero does.
	revisionLimit = max(revisionLimit, 0)
	var listOpts []client.ListOption
	var usingRevision *common.Revision
	var kind common.DefinitionType
	defMeta, ok := def.(metav1.Object)
	if !ok {
		return fmt.Errorf("unsupported type %T", def)
	}

	switch definition := def.(type) {
	case *v1beta1.ComponentDefinition:
		kind = common.ComponentType
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelComponentDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision
	case *v1beta1.TraitDefinition:
		kind = common.TraitType
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelTraitDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision
	case *v1beta1.PolicyDefinition:
		kind = common.PolicyType
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelPolicyDefinitionName: definition.Name},
		}
		usingRevision = definition.Status.LatestRevision
	case *v1beta1.WorkflowStepDefinition:
		kind = common.WorkflowStepType
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelWorkflowStepDefinitionName: definition.Name}}
		usingRevision = definition.Status.LatestRevision
	case *v1beta1.SourceDefinition:
		kind = common.SourceType
		listOpts = []client.ListOption{
			client.InNamespace(definition.Namespace),
			client.MatchingLabels{oam.LabelSourceDefinitionName: definition.Name}}
		usingRevision = definition.Status.LatestRevision
	}

	if usingRevision == nil {
		return nil
	}

	defRevList := new(v1beta1.DefinitionRevisionList)
	if err := cli.List(ctx, defRevList, listOpts...); err != nil {
		return err
	}
	if len(defRevList.Items)-revisionLimit-1 <= 0 {
		return nil
	}
	pinned, err := pinnedRevisions(ctx, cli, kind, defMeta, defRevList.Items)
	if err != nil {
		return err
	}
	var candidates []v1beta1.DefinitionRevision
	for _, rev := range defRevList.Items {
		if rev.Name != usingRevision.Name && !pinned[rev.Name] {
			candidates = append(candidates, rev)
		}
	}
	needKill := len(candidates) - revisionLimit
	if needKill <= 0 {
		return nil
	}
	klog.InfoS("cleanup old definitionRevision", "needKillNum", needKill, "pinned", len(pinned))

	sort.Sort(historiesByRevision(candidates))
	for _, rev := range candidates[:needKill] {
		if err := cli.Delete(ctx, rev.DeepCopy()); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// pinnedRevisions returns the names of revs that an Application pins as <definition>@v<n>. A
// definition in the system namespace may be pinned from any namespace, any other only from its
// own.
func pinnedRevisions(ctx context.Context, cli client.Client, kind common.DefinitionType, def metav1.Object, revs []v1beta1.DefinitionRevision) (map[string]bool, error) {
	var reader client.Reader = cli
	indexed := velacache.DefinitionPinIndexed
	if sharding.EnableSharding && LiveReader != nil {
		reader, indexed = LiveReader, false
	}
	var scope []client.ListOption
	if def.GetNamespace() != oam.SystemDefinitionNamespace {
		scope = append(scope, client.InNamespace(def.GetNamespace()))
	}
	pinOf := func(rev v1beta1.DefinitionRevision) (string, bool) {
		version, ok := strings.CutPrefix(rev.Name, def.GetName()+"-v")
		return def.GetName() + "@v" + version, ok
	}

	pinned := map[string]bool{}
	if indexed {
		for _, rev := range revs {
			pin, ok := pinOf(rev)
			if !ok {
				continue
			}
			apps := &v1beta1.ApplicationList{}
			opts := append([]client.ListOption{client.MatchingFields{velacache.DefinitionPinIndex: velacache.PinKey(kind, pin)}}, scope...)
			if err := reader.List(ctx, apps, opts...); err != nil {
				return nil, err
			}
			pinned[rev.Name] = len(apps.Items) > 0
		}
		return pinned, nil
	}

	apps := &v1beta1.ApplicationList{}
	if err := reader.List(ctx, apps, append([]client.ListOption{client.UnsafeDisableDeepCopy}, scope...)...); err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for i := range apps.Items {
		for _, k := range velacache.DefinitionPinsOf(&apps.Items[i]) {
			keys[k] = true
		}
	}
	for _, rev := range revs {
		if pin, ok := pinOf(rev); ok && keys[velacache.PinKey(kind, pin)] {
			pinned[rev.Name] = true
		}
	}
	return pinned, nil
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
			klog.ErrorS(err, "Could not update Definition Details")
			record.Event(definition, event.Warning("cannot update the definition status", err))
			return nil, &ctrl.Result{}, util.PatchCondition(ctx, cli, definition,
				condition.ReconcileError(fmt.Errorf(util.ErrUpdateComponentDefinition, definition.GetName(), err)))
		}
		klog.InfoS("Successfully updated the status.latestRevision of the definition", "Definition", klog.KRef(definition.GetNamespace(), definition.GetName()),
			"Name", defRev.Name, "Revision", defRev.Spec.Revision, "RevisionHash", defRev.Spec.RevisionHash)
	}

	// Recorded on every reconcile, after any create, so a definition that never changes is
	// recorded too, and a reconcile running in between sees the revision before its number.
	if err := recordRevisionNumber(ctx, cli, definition.GetNamespace(), defRev.Spec.DefinitionType, definition.GetName(), defRev.Spec.Revision, defRev.Spec.RevisionHash); err != nil {
		klog.InfoS("Failed to record the definition's revision number", "definition", klog.KObj(definition), "err", err)
		record.Event(definition, event.Warning("failed to record the DefinitionRevision number", err))
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
	case *v1beta1.SourceDefinition:
		labelKey = oam.LabelSourceDefinitionName
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
