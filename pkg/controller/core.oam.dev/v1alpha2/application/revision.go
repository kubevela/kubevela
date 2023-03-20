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

package application

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktypes "k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/compression"

	"github.com/kubevela/pkg/controller/sharding"
	monitorContext "github.com/kubevela/pkg/monitor/context"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	pkgutils "github.com/oam-dev/kubevela/pkg/utils"
)

type contextKey int

const (
	// ConfigMapKeyComponents is the key in ConfigMap Data field for containing data of components
	ConfigMapKeyComponents = "components"
	// ConfigMapKeyPolicy is the key in ConfigMap Data field for containing data of policies
	ConfigMapKeyPolicy = "policies"
	// ManifestKeyWorkload is the key in Component Manifest for containing workload cr.
	ManifestKeyWorkload = "StandardWorkload"
	// ManifestKeyTraits is the key in Component Manifest for containing Trait cr.
	ManifestKeyTraits = "Traits"
	// ManifestKeyScopes is the key in Component Manifest for containing scope cr reference.
	ManifestKeyScopes = "Scopes"
)

const rolloutTraitName = "rollout"

// contextWithComponent records ApplicationComponent in context
func contextWithComponent(ctx context.Context, component *common.ApplicationComponent) context.Context {
	return context.WithValue(ctx, ComponentContextKey, component)
}

// componentInContext extract ApplicationComponent from context
func componentInContext(ctx context.Context) *common.ApplicationComponent {
	comp, _ := ctx.Value(ComponentContextKey).(*common.ApplicationComponent)
	return comp
}

func _containsRolloutTrait(ctx context.Context) bool {
	comp := componentInContext(ctx)
	if comp != nil {
		for _, trait := range comp.Traits {
			if trait.Type == rolloutTraitName {
				return true
			}
		}
	}
	return false
}

var (
	// DisableAllComponentRevision disable component revision creation
	DisableAllComponentRevision = false
	// DisableAllApplicationRevision disable application revision creation
	DisableAllApplicationRevision = false
)

func contextWithComponentNamespace(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, ComponentNamespaceContextKey, ns)
}

func componentNamespaceFromContext(ctx context.Context) string {
	ns, _ := ctx.Value(ComponentNamespaceContextKey).(string)
	return ns
}

func contextWithReplicaKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, ReplicaKeyContextKey, key)
}

func replicaKeyFromContext(ctx context.Context) string {
	key, _ := ctx.Value(ReplicaKeyContextKey).(string)
	return key
}

func (h *AppHandler) getComponentRevisionNamespace(ctx context.Context) string {
	if ns, ok := ctx.Value(ComponentNamespaceContextKey).(string); ok && ns != "" {
		return ns
	}
	return h.app.Namespace
}

func (h *AppHandler) createResourcesConfigMap(ctx context.Context,
	appRev *v1beta1.ApplicationRevision,
	comps []*types.ComponentManifest,
	policies []*unstructured.Unstructured) error {

	components := map[string]interface{}{}
	for _, c := range comps {
		components[c.Name] = SprintComponentManifest(c)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appRev.Name,
			Namespace: appRev.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(appRev, v1beta1.ApplicationRevisionGroupVersionKind),
			},
		},
		Data: map[string]string{
			ConfigMapKeyComponents: string(util.MustJSONMarshal(components)),
			ConfigMapKeyPolicy:     string(util.MustJSONMarshal(policies)),
		},
	}
	err := h.r.Client.Get(ctx, client.ObjectKey{Name: appRev.Name, Namespace: appRev.Namespace}, &corev1.ConfigMap{})
	if err == nil {
		return nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return h.r.Client.Create(ctx, cm)
}

// SprintComponentManifest formats and returns the resulting string.
func SprintComponentManifest(cm *types.ComponentManifest) string {
	if cm.StandardWorkload.GetName() == "" {
		cm.StandardWorkload.SetName(cm.Name)
	}
	if cm.StandardWorkload.GetNamespace() == "" {
		cm.StandardWorkload.SetNamespace(cm.Namespace)
	}
	cl := map[string]interface{}{
		ManifestKeyWorkload: string(util.MustJSONMarshal(cm.StandardWorkload)),
	}

	trs := []string{}
	for _, tr := range cm.Traits {
		if tr.GetName() == "" {
			tr.SetName(cm.Name)
		}
		if tr.GetNamespace() == "" {
			tr.SetNamespace(cm.Namespace)
		}
		trs = append(trs, string(util.MustJSONMarshal(tr)))
	}
	cl[ManifestKeyTraits] = trs
	cl[ManifestKeyScopes] = cm.Scopes
	return string(util.MustJSONMarshal(cl))
}

// PrepareCurrentAppRevision will generate a pure revision without metadata and rendered result
// the generated revision will be compare with the last revision to see if there's any difference.
func (h *AppHandler) PrepareCurrentAppRevision(ctx context.Context, af *appfile.Appfile) error {
	if ctx, ok := ctx.(monitorContext.Context); ok {
		subCtx := ctx.Fork("prepare-current-appRevision", monitorContext.DurationMetric(func(v float64) {
			metrics.AppReconcileStageDurationHistogram.WithLabelValues("prepare-current-apprev").Observe(v)
		}))
		defer subCtx.Commit("finish prepare current appRevision")
	}

	if af.AppRevision != nil {
		h.isNewRevision = false
		h.latestAppRev = af.AppRevision
		h.currentAppRev = af.AppRevision
		h.currentRevHash = af.AppRevisionHash
		return nil
	}

	appRev, appRevisionHash, err := h.gatherRevisionSpec(af)
	if err != nil {
		return err
	}
	h.currentAppRev = appRev
	h.currentRevHash = appRevisionHash
	if err := h.getLatestAppRevision(ctx); err != nil {
		return err
	}

	var needGenerateRevision bool
	h.isNewRevision, needGenerateRevision, err = h.currentAppRevIsNew(ctx)
	if err != nil {
		return err
	}
	if h.isNewRevision && needGenerateRevision {
		h.currentAppRev.Name, _ = utils.GetAppNextRevision(h.app)
	}

	// MUST pass app revision name to appfile
	// appfile depends it to render resources and do health checking
	af.AppRevisionName = h.currentAppRev.Name
	af.AppRevisionHash = h.currentRevHash
	return nil
}

// gatherRevisionSpec will gather all revision spec without metadata and rendered result.
// the gathered Revision spec will be enough to calculate the hash and compare with the old revision
func (h *AppHandler) gatherRevisionSpec(af *appfile.Appfile) (*v1beta1.ApplicationRevision, string, error) {
	copiedApp := h.app.DeepCopy()
	// We better to remove all object status in the appRevision
	copiedApp.Status = common.AppStatus{}
	appRev := &v1beta1.ApplicationRevision{
		Spec: v1beta1.ApplicationRevisionSpec{
			ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
				Application:             *copiedApp,
				ComponentDefinitions:    make(map[string]*v1beta1.ComponentDefinition),
				WorkloadDefinitions:     make(map[string]v1beta1.WorkloadDefinition),
				TraitDefinitions:        make(map[string]*v1beta1.TraitDefinition),
				ScopeDefinitions:        make(map[string]v1beta1.ScopeDefinition),
				PolicyDefinitions:       make(map[string]v1beta1.PolicyDefinition),
				WorkflowStepDefinitions: make(map[string]*v1beta1.WorkflowStepDefinition),
				ScopeGVK:                make(map[string]metav1.GroupVersionKind),
				Policies:                make(map[string]v1alpha1.Policy),
			},
		},
	}
	for _, w := range af.Workloads {
		if w == nil {
			continue
		}
		if w.FullTemplate.ComponentDefinition != nil {
			cd := w.FullTemplate.ComponentDefinition.DeepCopy()
			cd.Status = v1beta1.ComponentDefinitionStatus{}
			appRev.Spec.ComponentDefinitions[w.FullTemplate.ComponentDefinition.Name] = cd.DeepCopy()
		}
		if w.FullTemplate.WorkloadDefinition != nil {
			wd := w.FullTemplate.WorkloadDefinition.DeepCopy()
			wd.Status = v1beta1.WorkloadDefinitionStatus{}
			appRev.Spec.WorkloadDefinitions[w.FullTemplate.WorkloadDefinition.Name] = *wd
		}
		for _, t := range w.Traits {
			if t == nil {
				continue
			}
			if t.FullTemplate.TraitDefinition != nil {
				td := t.FullTemplate.TraitDefinition.DeepCopy()
				td.Status = v1beta1.TraitDefinitionStatus{}
				appRev.Spec.TraitDefinitions[t.FullTemplate.TraitDefinition.Name] = td.DeepCopy()
			}
		}
		for _, s := range w.ScopeDefinition {
			if s == nil {
				continue
			}
			appRev.Spec.ScopeDefinitions[s.Name] = *s.DeepCopy()
		}
		for _, s := range w.Scopes {
			appRev.Spec.ScopeGVK[s.ResourceVersion] = s.GVK
		}
	}
	for _, p := range af.PolicyWorkloads {
		if p == nil || p.FullTemplate == nil {
			continue
		}
		if p.FullTemplate.PolicyDefinition != nil {
			pd := p.FullTemplate.PolicyDefinition.DeepCopy()
			pd.Status = v1beta1.PolicyDefinitionStatus{}
			appRev.Spec.PolicyDefinitions[p.FullTemplate.PolicyDefinition.Name] = *pd
		}
	}
	for name, def := range af.RelatedComponentDefinitions {
		appRev.Spec.ComponentDefinitions[name] = def.DeepCopy()
	}
	for name, def := range af.RelatedTraitDefinitions {
		appRev.Spec.TraitDefinitions[name] = def.DeepCopy()
	}
	for name, def := range af.RelatedWorkflowStepDefinitions {
		appRev.Spec.WorkflowStepDefinitions[name] = def.DeepCopy()
	}
	for name, po := range af.ExternalPolicies {
		appRev.Spec.Policies[name] = *po
	}
	var err error
	if appRev.Spec.ReferredObjects, err = component.ConvertUnstructuredsToReferredObjects(af.ReferredObjects); err != nil {
		return nil, "", errors.Wrapf(err, "failed to marshal referred object")
	}
	appRev.Spec.Workflow = af.ExternalWorkflow

	appRevisionHash, err := ComputeAppRevisionHash(appRev)
	if err != nil {
		klog.ErrorS(err, "Failed to compute hash of appRevision for application", "application", klog.KObj(h.app))
		return appRev, "", errors.Wrapf(err, "failed to compute app revision hash")
	}
	return appRev, appRevisionHash, nil
}

func (h *AppHandler) getLatestAppRevision(ctx context.Context) error {
	if DisableAllApplicationRevision {
		return nil
	}
	if h.app.Status.LatestRevision == nil || len(h.app.Status.LatestRevision.Name) == 0 {
		return nil
	}
	latestRevName := h.app.Status.LatestRevision.Name
	latestAppRev := &v1beta1.ApplicationRevision{}
	if err := h.r.Get(ctx, client.ObjectKey{Name: latestRevName, Namespace: h.app.Namespace}, latestAppRev); err != nil {
		klog.ErrorS(err, "Failed to get latest app revision", "appRevisionName", latestRevName)
		return errors.Wrapf(err, "fail to get latest app revision %s", latestRevName)
	}
	h.latestAppRev = latestAppRev
	return nil
}

// ComputeAppRevisionHash computes a single hash value for an appRevision object
// Spec of Application/WorkloadDefinitions/ComponentDefinitions/TraitDefinitions/ScopeDefinitions will be taken into compute
func ComputeAppRevisionHash(appRevision *v1beta1.ApplicationRevision) (string, error) {
	// Calculate Hash for New Mode with workflow and policy
	revHash := struct {
		ApplicationSpecHash        string
		WorkloadDefinitionHash     map[string]string
		ComponentDefinitionHash    map[string]string
		TraitDefinitionHash        map[string]string
		ScopeDefinitionHash        map[string]string
		PolicyDefinitionHash       map[string]string
		WorkflowStepDefinitionHash map[string]string
		PolicyHash                 map[string]string
		WorkflowHash               string
		ReferredObjectsHash        string
	}{
		WorkloadDefinitionHash:     make(map[string]string),
		ComponentDefinitionHash:    make(map[string]string),
		TraitDefinitionHash:        make(map[string]string),
		ScopeDefinitionHash:        make(map[string]string),
		PolicyDefinitionHash:       make(map[string]string),
		WorkflowStepDefinitionHash: make(map[string]string),
		PolicyHash:                 make(map[string]string),
	}
	var err error
	revHash.ApplicationSpecHash, err = utils.ComputeSpecHash(appRevision.Spec.Application.Spec)
	if err != nil {
		return "", err
	}
	for key, wd := range appRevision.Spec.WorkloadDefinitions {
		hash, err := utils.ComputeSpecHash(&wd.Spec)
		if err != nil {
			return "", err
		}
		revHash.WorkloadDefinitionHash[key] = hash
	}
	for key, cd := range appRevision.Spec.ComponentDefinitions {
		hash, err := utils.ComputeSpecHash(&cd.Spec)
		if err != nil {
			return "", err
		}
		revHash.ComponentDefinitionHash[key] = hash
	}
	for key, td := range appRevision.Spec.TraitDefinitions {
		hash, err := utils.ComputeSpecHash(&td.Spec)
		if err != nil {
			return "", err
		}
		revHash.TraitDefinitionHash[key] = hash
	}
	for key, sd := range appRevision.Spec.ScopeDefinitions {
		hash, err := utils.ComputeSpecHash(&sd.Spec)
		if err != nil {
			return "", err
		}
		revHash.ScopeDefinitionHash[key] = hash
	}
	for key, pd := range appRevision.Spec.PolicyDefinitions {
		hash, err := utils.ComputeSpecHash(&pd.Spec)
		if err != nil {
			return "", err
		}
		revHash.PolicyDefinitionHash[key] = hash
	}
	for key, wd := range appRevision.Spec.WorkflowStepDefinitions {
		hash, err := utils.ComputeSpecHash(&wd.Spec)
		if err != nil {
			return "", err
		}
		revHash.WorkflowStepDefinitionHash[key] = hash
	}
	for key, po := range appRevision.Spec.Policies {
		hash, err := utils.ComputeSpecHash(po.Properties)
		if err != nil {
			return "", err
		}
		revHash.PolicyHash[key] = hash + po.Type
	}
	if appRevision.Spec.Workflow != nil {
		revHash.WorkflowHash, err = utils.ComputeSpecHash(appRevision.Spec.Workflow.Steps)
		if err != nil {
			return "", err
		}
	}
	revHash.ReferredObjectsHash, err = utils.ComputeSpecHash(appRevision.Spec.ReferredObjects)
	if err != nil {
		return "", err
	}
	return utils.ComputeSpecHash(&revHash)
}

// currentAppRevIsNew check application revision already exist or not
func (h *AppHandler) currentAppRevIsNew(ctx context.Context) (bool, bool, error) {
	// the last revision doesn't exist.
	if h.app.Status.LatestRevision == nil || DisableAllApplicationRevision {
		return true, true, nil
	}

	isLatestRev := deepEqualAppInRevision(h.latestAppRev, h.currentAppRev)
	if metav1.HasAnnotation(h.app.ObjectMeta, oam.AnnotationAutoUpdate) {
		isLatestRev = h.app.Status.LatestRevision.RevisionHash == h.currentRevHash && DeepEqualRevision(h.latestAppRev, h.currentAppRev)
	}
	if h.latestAppRev != nil && oam.GetPublishVersion(h.app) != oam.GetPublishVersion(h.latestAppRev) {
		isLatestRev = false
	}

	// diff the latest revision first
	if isLatestRev {
		appSpec := h.currentAppRev.Spec.Application.Spec
		traitDef := h.currentAppRev.Spec.TraitDefinitions
		workflowStepDef := h.currentAppRev.Spec.WorkflowStepDefinitions
		h.currentAppRev = h.latestAppRev.DeepCopy()
		h.currentRevHash = h.app.Status.LatestRevision.RevisionHash
		h.currentAppRev.Spec.Application.Spec = appSpec
		h.currentAppRev.Spec.TraitDefinitions = traitDef
		h.currentAppRev.Spec.WorkflowStepDefinitions = workflowStepDef
		return false, false, nil
	}

	revs, err := GetAppRevisions(ctx, h.r.Client, h.app.Name, h.app.Namespace)
	if err != nil {
		klog.ErrorS(err, "Failed to list app revision", "appName", h.app.Name)
		return false, false, errors.Wrap(err, "failed to list app revision")
	}

	for _, _rev := range revs {
		rev := _rev.DeepCopy()
		if rev.GetLabels()[oam.LabelAppRevisionHash] == h.currentRevHash &&
			DeepEqualRevision(rev, h.currentAppRev) &&
			oam.GetPublishVersion(rev) == oam.GetPublishVersion(h.app) {
			// we set currentAppRev to existRevision
			h.currentAppRev = rev
			return true, false, nil
		}
	}

	// if reach here, it has different spec
	return true, true, nil
}

// DeepEqualRevision will compare the spec of Application and Definition to see if the Application is the same revision
// Spec of AC and Component will not be compared as they are generated by the application and definitions
// Note the Spec compare can only work when the RawExtension are decoded well in the RawExtension.Object instead of in RawExtension.Raw(bytes)
func DeepEqualRevision(old, new *v1beta1.ApplicationRevision) bool {
	if len(old.Spec.WorkloadDefinitions) != len(new.Spec.WorkloadDefinitions) {
		return false
	}
	oldTraitDefinitions := old.Spec.TraitDefinitions
	newTraitDefinitions := new.Spec.TraitDefinitions
	if len(oldTraitDefinitions) != len(newTraitDefinitions) {
		return false
	}
	if len(old.Spec.ComponentDefinitions) != len(new.Spec.ComponentDefinitions) {
		return false
	}
	if len(old.Spec.ScopeDefinitions) != len(new.Spec.ScopeDefinitions) {
		return false
	}
	for key, wd := range new.Spec.WorkloadDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.WorkloadDefinitions[key].Spec, wd.Spec) {
			return false
		}
	}
	for key, cd := range new.Spec.ComponentDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.ComponentDefinitions[key].Spec, cd.Spec) {
			return false
		}
	}
	for key, td := range newTraitDefinitions {
		if !apiequality.Semantic.DeepEqual(oldTraitDefinitions[key].Spec, td.Spec) {
			return false
		}
	}
	for key, sd := range new.Spec.ScopeDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.ScopeDefinitions[key].Spec, sd.Spec) {
			return false
		}
	}
	return deepEqualAppInRevision(old, new)
}

func deepEqualPolicy(old, new v1alpha1.Policy) bool {
	return old.Type == new.Type && apiequality.Semantic.DeepEqual(old.Properties, new.Properties)
}

func deepEqualWorkflow(old, new workflowv1alpha1.Workflow) bool {
	return apiequality.Semantic.DeepEqual(old.Steps, new.Steps)
}

const velaVersionNumberToCompareWorkflow = "v1.5.7"

func deepEqualAppSpec(old, new *v1beta1.Application) bool {
	oldSpec, newSpec := old.Spec.DeepCopy(), new.Spec.DeepCopy()
	// legacy code: KubeVela version before v1.5.7 & v1.6.0-alpha.4 does not
	// record workflow in application spec in application revision. The comparison
	// need to bypass the equality check of workflow to prevent unintended rerun
	curVerNum := k8s.GetAnnotation(old, oam.AnnotationKubeVelaVersion)
	publishVersion := k8s.GetAnnotation(old, oam.AnnotationPublishVersion)
	if publishVersion == "" && curVerNum != "" {
		cmpVer, _ := version.NewVersion(velaVersionNumberToCompareWorkflow)
		if curVer, err := version.NewVersion(curVerNum); err == nil && curVer.LessThan(cmpVer) {
			oldSpec.Workflow = nil
			newSpec.Workflow = nil
		}
	}
	return apiequality.Semantic.DeepEqual(oldSpec, newSpec)
}

func deepEqualAppInRevision(old, new *v1beta1.ApplicationRevision) bool {
	if len(old.Spec.Policies) != len(new.Spec.Policies) {
		return false
	}
	for key, po := range new.Spec.Policies {
		if !deepEqualPolicy(old.Spec.Policies[key], po) {
			return false
		}
	}
	if (old.Spec.Workflow == nil) != (new.Spec.Workflow == nil) {
		return false
	}
	if old.Spec.Workflow != nil && new.Spec.Workflow != nil {
		if !deepEqualWorkflow(*old.Spec.Workflow, *new.Spec.Workflow) {
			return false
		}
	}
	return deepEqualAppSpec(&old.Spec.Application, &new.Spec.Application)
}

// HandleComponentsRevision manages Component revisions
// 1. if update component create a new component Revision
// 2. check all componentTrait  rely on componentRevName, if yes fill it
func (h *AppHandler) HandleComponentsRevision(ctx context.Context, compManifests []*types.ComponentManifest) error {
	if DisableAllComponentRevision {
		return nil
	}

	for _, cm := range compManifests {

		// external revision specified
		if len(cm.ExternalRevision) != 0 {
			if err := h.handleComponentRevisionNameSpecified(ctx, cm); err != nil {
				return err
			}
			continue
		}

		if err := h.handleComponentRevisionNameUnspecified(ctx, cm); err != nil {
			return err
		}

	}
	return nil
}

// handleComponentRevisionNameSpecified create controllerRevision which use specified revisionName.
// If the controllerRevision already exist, we just return
func (h *AppHandler) handleComponentRevisionNameSpecified(ctx context.Context, comp *types.ComponentManifest) error {
	revisionName := comp.ExternalRevision
	cr := &appsv1.ControllerRevision{}

	if err := h.r.Client.Get(auth.ContextWithUserInfo(ctx, h.app), client.ObjectKey{Namespace: h.getComponentRevisionNamespace(ctx), Name: revisionName}, cr); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get controllerRevision:%s", revisionName)
		}

		// we should create one
		hash, err := ComputeComponentRevisionHash(comp)
		if err != nil {
			return err
		}
		comp.RevisionHash = hash
		comp.RevisionName = revisionName
		if err := h.createControllerRevision(ctx, comp); err != nil {
			return err
		}

		// when controllerRevision not exist handle replace context.RevisionName
		for _, trait := range comp.Traits {
			if err := replaceComponentRevisionContext(trait, comp.RevisionName); err != nil {
				return err
			}
		}

		return nil
	}

	comp.RevisionHash = cr.GetLabels()[oam.LabelComponentRevisionHash]
	comp.RevisionName = revisionName

	for _, trait := range comp.Traits {
		if err := replaceComponentRevisionContext(trait, comp.RevisionName); err != nil {
			return err
		}
	}

	return nil
}

// handleComponentRevisionNameUnspecified create new controllerRevision when external revision name unspecified
func (h *AppHandler) handleComponentRevisionNameUnspecified(ctx context.Context, comp *types.ComponentManifest) error {
	hash, err := ComputeComponentRevisionHash(comp)
	if err != nil {
		return err
	}
	comp.RevisionHash = hash

	crList := &appsv1.ControllerRevisionList{}
	listOpts := []client.ListOption{client.MatchingLabels{
		oam.LabelControllerRevisionComponent: pkgutils.EscapeResourceNameToLabelValue(comp.Name),
	}, client.InNamespace(h.getComponentRevisionNamespace(ctx))}
	if err := h.r.List(auth.ContextWithUserInfo(ctx, h.app), crList, listOpts...); err != nil {
		return err
	}

	var maxRevisionNum int64
	needNewRevision := true
	for _, existingCR := range crList.Items {
		if existingCR.Revision > maxRevisionNum {
			maxRevisionNum = existingCR.Revision
		}
		if existingCR.GetLabels()[oam.LabelComponentRevisionHash] == comp.RevisionHash {
			existingComp, err := util.RawExtension2Component(existingCR.Data)
			if err != nil {
				return err
			}
			// let componentManifest2Component func replace context.Name's placeHolder to guarantee content of them to be same.
			comp.RevisionName = existingCR.GetName()
			currentComp, err := componentManifest2Component(comp)
			if err != nil {
				return err
			}
			// further check whether it's truly identical, even hash value is equal
			if checkComponentSpecEqual(existingComp, currentComp) {
				comp.RevisionName = existingCR.GetName()
				// found identical revision already exisits
				// skip creating new one
				needNewRevision = false
				break
			}
		}
	}
	if needNewRevision {
		comp.RevisionName = utils.ConstructRevisionName(comp.Name, maxRevisionNum+1)
		if err := h.createControllerRevision(ctx, comp); err != nil {
			return err
		}
	}
	for _, trait := range comp.Traits {
		if err := replaceComponentRevisionContext(trait, comp.RevisionName); err != nil {
			return err
		}
	}

	return nil
}

func checkComponentSpecEqual(a, b *v1alpha2.Component) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	au, err := util.RawExtension2Unstructured(&a.Spec.Workload)
	if err != nil {
		return false
	}
	bu, err := util.RawExtension2Unstructured(&b.Spec.Workload)
	if err != nil {
		return false
	}
	if !reflect.DeepEqual(au.Object["spec"], bu.Object["spec"]) {
		return false
	}
	return reflect.DeepEqual(a.Spec.Helm, b.Spec.Helm)
}

// ComputeComponentRevisionHash to compute component hash
func ComputeComponentRevisionHash(comp *types.ComponentManifest) (string, error) {
	compRevisionHash := struct {
		WorkloadHash          string
		PackagedResourcesHash []string
	}{}
	wl := comp.StandardWorkload.DeepCopy()
	if wl != nil {
		// Only calculate spec for component revision
		spec := wl.Object["spec"]
		hash, err := utils.ComputeSpecHash(spec)
		if err != nil {
			return "", err
		}
		compRevisionHash.WorkloadHash = hash
	}

	// take packaged workload resources into account because they determine the workload
	compRevisionHash.PackagedResourcesHash = make([]string, len(comp.PackagedWorkloadResources))
	for i, v := range comp.PackagedWorkloadResources {
		hash, err := utils.ComputeSpecHash(v)
		if err != nil {
			return "", err
		}
		compRevisionHash.PackagedResourcesHash[i] = hash
	}
	return utils.ComputeSpecHash(&compRevisionHash)
}

// createControllerRevision records snapshot of a component
func (h *AppHandler) createControllerRevision(ctx context.Context, cm *types.ComponentManifest) error {
	comp, err := componentManifest2Component(cm)
	if err != nil {
		return err
	}
	revision, _ := utils.ExtractRevision(cm.RevisionName)
	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.RevisionName,
			Namespace: h.getComponentRevisionNamespace(ctx),
			Labels: map[string]string{
				oam.LabelAppComponent:                pkgutils.EscapeResourceNameToLabelValue(cm.Name),
				oam.LabelAppCluster:                  multicluster.ClusterNameInContext(ctx),
				oam.LabelAppEnv:                      envbinding.EnvNameInContext(ctx),
				oam.LabelControllerRevisionComponent: pkgutils.EscapeResourceNameToLabelValue(cm.Name),
				oam.LabelComponentRevisionHash:       cm.RevisionHash,
			},
		},
		Revision: int64(revision),
		Data:     *util.Object2RawExtension(comp),
	}
	common.NewOAMObjectReferenceFromObject(cm.StandardWorkload).AddLabelsToObject(cr)
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.LegacyComponentRevision) && !_containsRolloutTrait(ctx) {
		return nil
	}
	return h.resourceKeeper.DispatchComponentRevision(ctx, cr)
}

func componentManifest2Component(cm *types.ComponentManifest) (*v1alpha2.Component, error) {
	component := &v1alpha2.Component{}
	component.SetGroupVersionKind(v1alpha2.ComponentGroupVersionKind)
	component.SetName(cm.Name)
	wl := &unstructured.Unstructured{}
	if cm.StandardWorkload != nil {
		// use revision name replace compRev placeHolder
		if err := replaceComponentRevisionContext(cm.StandardWorkload, cm.RevisionName); err != nil {
			return nil, err
		}
		wl = cm.StandardWorkload.DeepCopy()
		util.RemoveLabels(wl, []string{oam.LabelAppRevision})
	}
	component.Spec.Workload = *util.Object2RawExtension(wl)
	if len(cm.PackagedWorkloadResources) > 0 {
		helm := &common.Helm{}
		for _, helmResource := range cm.PackagedWorkloadResources {
			if helmResource.GetKind() == helmapi.HelmReleaseGVK.Kind {
				helm.Release = *util.Object2RawExtension(helmResource)
			}
			if helmResource.GetKind() == helmapi.HelmRepositoryGVK.Kind {
				helm.Repository = *util.Object2RawExtension(helmResource)
			}
		}
		component.Spec.Helm = helm
	}
	return component, nil
}

// FinalizeAndApplyAppRevision finalise AppRevision object and apply it
func (h *AppHandler) FinalizeAndApplyAppRevision(ctx context.Context) error {
	if DisableAllApplicationRevision {
		return nil
	}

	if ctx, ok := ctx.(monitorContext.Context); ok {
		subCtx := ctx.Fork("apply-app-revision", monitorContext.DurationMetric(func(v float64) {
			metrics.AppReconcileStageDurationHistogram.WithLabelValues("apply-apprev").Observe(v)
		}))
		defer subCtx.Commit("finish apply app revision")
	}
	appRev := h.currentAppRev
	appRev.Namespace = h.app.Namespace
	appRev.SetGroupVersionKind(v1beta1.ApplicationRevisionGroupVersionKind)
	// pass application's annotations & labels to app revision
	appRev.SetAnnotations(h.app.GetAnnotations())
	delete(appRev.Annotations, oam.AnnotationLastAppliedConfiguration)
	appRev.SetLabels(h.app.GetLabels())
	util.AddLabels(appRev, map[string]string{
		oam.LabelAppName:         h.app.GetName(),
		oam.LabelAppRevisionHash: h.currentRevHash,
	})
	// ApplicationRevision must use Application as ctrl-owner
	appRev.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       v1beta1.ApplicationKind,
		Name:       h.app.Name,
		UID:        h.app.UID,
		Controller: pointer.Bool(true),
	}})
	sharding.PropagateScheduledShardIDLabel(h.app, appRev)

	gotAppRev := &v1beta1.ApplicationRevision{}
	if err := h.r.Get(ctx, client.ObjectKey{Name: appRev.Name, Namespace: appRev.Namespace}, gotAppRev); err != nil {
		if apierrors.IsNotFound(err) {
			return h.r.Create(ctx, appRev)
		}
		return err
	}
	if apiequality.Semantic.DeepEqual(gotAppRev.Spec, appRev.Spec) &&
		apiequality.Semantic.DeepEqual(gotAppRev.GetLabels(), appRev.GetLabels()) &&
		apiequality.Semantic.DeepEqual(gotAppRev.GetAnnotations(), appRev.GetAnnotations()) {
		return nil
	}
	appRev.ResourceVersion = gotAppRev.ResourceVersion

	// Set compression types (if enabled)
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.GzipApplicationRevision) {
		appRev.Spec.Compression.SetType(compression.Gzip)
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.ZstdApplicationRevision) {
		appRev.Spec.Compression.SetType(compression.Zstd)
	}

	return h.r.Update(ctx, appRev)
}

// UpdateAppLatestRevisionStatus only call to update app's latest revision status after applying manifests successfully
// otherwise it will override previous revision which is used during applying to do GC jobs
func (h *AppHandler) UpdateAppLatestRevisionStatus(ctx context.Context) error {
	if DisableAllApplicationRevision {
		return nil
	}
	if !h.isNewRevision {
		// skip update if app revision is not changed
		return nil
	}
	if ctx, ok := ctx.(monitorContext.Context); ok {
		subCtx := ctx.Fork("update-apprev-status", monitorContext.DurationMetric(func(v float64) {
			metrics.AppReconcileStageDurationHistogram.WithLabelValues("update-apprev-status").Observe(v)
		}))
		defer subCtx.Commit("application revision status updated")
	}
	revName := h.currentAppRev.Name
	revNum, _ := util.ExtractRevisionNum(revName, "-")
	h.app.Status.LatestRevision = &common.Revision{
		Name:         h.currentAppRev.Name,
		Revision:     int64(revNum),
		RevisionHash: h.currentRevHash,
	}
	if err := h.r.patchStatus(ctx, h.app, common.ApplicationRendering); err != nil {
		klog.InfoS("Failed to update the latest appConfig revision to status", "application", klog.KObj(h.app),
			"latest revision", revName, "err", err)
		return err
	}
	klog.InfoS("Successfully update application latest revision status", "application", klog.KObj(h.app),
		"latest revision", revName)
	return nil
}

// cleanUpApplicationRevision check all appRevisions of the application, remove them if the number of them exceed the limit
func cleanUpApplicationRevision(ctx context.Context, h *AppHandler) error {
	if DisableAllApplicationRevision {
		return nil
	}
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-rev.apprev").Observe(time.Since(t).Seconds())
	}()
	sortedRevision, err := GetSortedAppRevisions(ctx, h.r.Client, h.app.Name, h.app.Namespace)
	if err != nil {
		return err
	}
	appRevisionInUse := gatherUsingAppRevision(h)
	needKill := len(sortedRevision) - h.r.appRevisionLimit - len(appRevisionInUse)
	if needKill <= 0 {
		return nil
	}
	klog.InfoS("Going to garbage collect app revisions", "limit", h.r.appRevisionLimit,
		"total", len(sortedRevision), "using", len(appRevisionInUse), "kill", needKill)

	for _, rev := range sortedRevision {
		if needKill <= 0 {
			break
		}
		// don't delete app revision in use
		if appRevisionInUse[rev.Name] {
			continue
		}
		if err := h.r.Delete(ctx, rev.DeepCopy()); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		needKill--
	}
	return nil
}

// gatherUsingAppRevision get all using appRevisions include app's status pointing to
func gatherUsingAppRevision(h *AppHandler) map[string]bool {
	usingRevision := map[string]bool{}
	if h.app.Status.LatestRevision != nil && len(h.app.Status.LatestRevision.Name) != 0 {
		usingRevision[h.app.Status.LatestRevision.Name] = true
	}
	return usingRevision
}

func replaceComponentRevisionContext(u *unstructured.Unstructured, compRevName string) error {
	str := string(util.JSONMarshal(u))
	if strings.Contains(str, process.ComponentRevisionPlaceHolder) {
		newStr := strings.ReplaceAll(str, process.ComponentRevisionPlaceHolder, compRevName)
		if err := json.Unmarshal([]byte(newStr), u); err != nil {
			return err
		}
	}
	return nil
}

func cleanUpWorkflowComponentRevision(ctx context.Context, h *AppHandler) error {
	if DisableAllComponentRevision {
		return nil
	}
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-rev.comprev").Observe(time.Since(t).Seconds())
	}()
	// collect component revision in use
	compRevisionInUse := map[string]map[string]struct{}{}
	ctx = auth.ContextWithUserInfo(ctx, h.app)
	for i, resource := range h.app.Status.AppliedResources {
		compName := resource.Name
		ns := resource.Namespace
		r := &unstructured.Unstructured{}
		r.GetObjectKind().SetGroupVersionKind(resource.GroupVersionKind())
		_ctx := multicluster.ContextWithClusterName(ctx, resource.Cluster)
		err := h.r.Get(_ctx, ktypes.NamespacedName{Name: compName, Namespace: ns}, r)
		notFound := apierrors.IsNotFound(err)
		if err != nil && !notFound {
			return errors.WithMessagef(err, "get applied resource index=%d", i)
		}
		if compRevisionInUse[compName] == nil {
			compRevisionInUse[compName] = map[string]struct{}{}
		}
		if notFound {
			continue
		}
		compRevision, ok := r.GetLabels()[oam.LabelAppComponentRevision]
		if ok {
			compRevisionInUse[compName][compRevision] = struct{}{}
		}
	}

	for _, curComp := range h.app.Status.AppliedResources {
		crList := &appsv1.ControllerRevisionList{}
		listOpts := []client.ListOption{client.MatchingLabels{
			oam.LabelControllerRevisionComponent: pkgutils.EscapeResourceNameToLabelValue(curComp.Name),
		}, client.InNamespace(h.getComponentRevisionNamespace(ctx))}
		_ctx := multicluster.ContextWithClusterName(ctx, curComp.Cluster)
		if err := h.r.List(_ctx, crList, listOpts...); err != nil {
			return err
		}
		needKill := len(crList.Items) - h.r.appRevisionLimit - len(compRevisionInUse[curComp.Name])
		if needKill < 1 {
			continue
		}
		sortedRevision := crList.Items
		sort.Sort(historiesByComponentRevision(sortedRevision))
		for _, rev := range sortedRevision {
			if needKill <= 0 {
				break
			}
			if _, inUse := compRevisionInUse[curComp.Name][rev.Name]; inUse {
				continue
			}
			_rev := rev.DeepCopy()
			oam.SetCluster(_rev, curComp.Cluster)
			if err := h.resourceKeeper.DeleteComponentRevision(_ctx, _rev); err != nil {
				return err
			}
			needKill--
		}
	}
	return nil
}

type historiesByComponentRevision []appsv1.ControllerRevision

func (h historiesByComponentRevision) Len() int      { return len(h) }
func (h historiesByComponentRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByComponentRevision) Less(i, j int) bool {
	ir, _ := util.ExtractRevisionNum(h[i].Name, "-")
	ij, _ := util.ExtractRevisionNum(h[j].Name, "-")
	return ir < ij
}

// UpdateApplicationRevisionStatus update application revision status
func (h *AppHandler) UpdateApplicationRevisionStatus(ctx context.Context, appRev *v1beta1.ApplicationRevision, wfStatus *common.WorkflowStatus) {
	if appRev == nil || DisableAllApplicationRevision {
		return
	}
	appRev.Status.Succeeded = wfStatus.Phase == workflowv1alpha1.WorkflowStateSucceeded
	appRev.Status.Workflow = wfStatus

	// Versioned the context backend values.
	if wfStatus.ContextBackend != nil {
		var cm corev1.ConfigMap
		if err := h.r.Client.Get(ctx, ktypes.NamespacedName{Namespace: wfStatus.ContextBackend.Namespace, Name: wfStatus.ContextBackend.Name}, &cm); err != nil {
			klog.Error(err, "[UpdateApplicationRevisionStatus] failed to load the context values", "ApplicationRevision", appRev.Name)
		}
		appRev.Status.WorkflowContext = cm.Data
	}

	if err := h.r.Client.Status().Update(ctx, appRev); err != nil {
		if logCtx, ok := ctx.(monitorContext.Context); ok {
			logCtx.Error(err, "[UpdateApplicationRevisionStatus] failed to update application revision status", "ApplicationRevision", appRev.Name)
		} else {
			klog.Error(err, "[UpdateApplicationRevisionStatus] failed to update application revision status", "ApplicationRevision", appRev.Name)
		}
	}
}

// GetAppRevisions get application revisions by label
func GetAppRevisions(ctx context.Context, cli client.Client, appName string, appNs string) ([]v1beta1.ApplicationRevision, error) {
	appRevisionList := new(v1beta1.ApplicationRevisionList)
	var err error
	if cache.OptimizeListOp {
		err = cli.List(ctx, appRevisionList, client.MatchingFields{cache.AppIndex: appNs + "/" + appName})
	} else {
		err = cli.List(ctx, appRevisionList, client.InNamespace(appNs), client.MatchingLabels{oam.LabelAppName: appName})
	}
	if err != nil {
		return nil, err
	}
	return appRevisionList.Items, nil
}

// GetSortedAppRevisions get application revisions by revision number
func GetSortedAppRevisions(ctx context.Context, cli client.Client, appName string, appNs string) ([]v1beta1.ApplicationRevision, error) {
	revs, err := GetAppRevisions(ctx, cli, appName, appNs)
	if err != nil {
		return nil, err
	}
	sort.Slice(revs, func(i, j int) bool {
		ir, _ := util.ExtractRevisionNum(revs[i].Name, "-")
		ij, _ := util.ExtractRevisionNum(revs[j].Name, "-")
		return ir < ij
	})
	return revs, nil
}
