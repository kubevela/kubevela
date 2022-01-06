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

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

type contextKey string

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
	// ComponentRevisionNamespaceContextKey is the key in context that defines the override namespace of component revision
	ComponentRevisionNamespaceContextKey = contextKey("component-revision-namespace")
)

func contextWithComponentRevisionNamespace(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, ComponentRevisionNamespaceContextKey, ns)
}

func (h *AppHandler) getComponentRevisionNamespace(ctx context.Context) string {
	if ns, ok := ctx.Value(ComponentRevisionNamespaceContextKey).(string); ok && ns != "" {
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
	copiedApp.Spec.Workflow = nil
	appRev := &v1beta1.ApplicationRevision{
		Spec: v1beta1.ApplicationRevisionSpec{
			Application:             *copiedApp,
			ComponentDefinitions:    make(map[string]v1beta1.ComponentDefinition),
			WorkloadDefinitions:     make(map[string]v1beta1.WorkloadDefinition),
			TraitDefinitions:        make(map[string]v1beta1.TraitDefinition),
			ScopeDefinitions:        make(map[string]v1beta1.ScopeDefinition),
			PolicyDefinitions:       make(map[string]v1beta1.PolicyDefinition),
			WorkflowStepDefinitions: make(map[string]v1beta1.WorkflowStepDefinition),
			ScopeGVK:                make(map[string]metav1.GroupVersionKind),

			// add an empty appConfig here just for compatible as old version kubevela need appconfig as required value
			ApplicationConfiguration: runtime.RawExtension{Raw: []byte(`{"apiVersion":"core.oam.dev/v1alpha2","kind":"ApplicationConfiguration"}`)},
		},
	}
	for _, w := range af.Workloads {
		if w == nil {
			continue
		}
		if w.FullTemplate.ComponentDefinition != nil {
			cd := w.FullTemplate.ComponentDefinition.DeepCopy()
			cd.Status = v1beta1.ComponentDefinitionStatus{}
			appRev.Spec.ComponentDefinitions[w.FullTemplate.ComponentDefinition.Name] = *cd
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
				appRev.Spec.TraitDefinitions[t.FullTemplate.TraitDefinition.Name] = *td
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
	for _, p := range af.Policies {
		if p == nil || p.FullTemplate == nil {
			continue
		}
		if p.FullTemplate.PolicyDefinition != nil {
			pd := p.FullTemplate.PolicyDefinition.DeepCopy()
			pd.Status = v1beta1.PolicyDefinitionStatus{}
			appRev.Spec.PolicyDefinitions[p.FullTemplate.PolicyDefinition.Name] = *pd
		}
	}

	appRevisionHash, err := ComputeAppRevisionHash(appRev)
	if err != nil {
		klog.ErrorS(err, "Failed to compute hash of appRevision for application", "application", klog.KObj(h.app))
		return appRev, "", errors.Wrapf(err, "failed to compute app revision hash")
	}
	return appRev, appRevisionHash, nil
}

func (h *AppHandler) getLatestAppRevision(ctx context.Context) error {
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
	// we first constructs a AppRevisionHash structure to store all the meaningful spec hashes
	// and avoid computing the annotations. Those fields are all read from k8s already so their
	// raw extension value are already byte array. Never include any in-memory objects.
	type AppRevisionHash struct {
		ApplicationSpecHash     string
		WorkloadDefinitionHash  map[string]string
		ComponentDefinitionHash map[string]string
		TraitDefinitionHash     map[string]string
		ScopeDefinitionHash     map[string]string
	}
	appRevisionHash := AppRevisionHash{
		WorkloadDefinitionHash:  make(map[string]string),
		ComponentDefinitionHash: make(map[string]string),
		TraitDefinitionHash:     make(map[string]string),
		ScopeDefinitionHash:     make(map[string]string),
	}
	var err error
	appRevisionHash.ApplicationSpecHash, err = utils.ComputeSpecHash(filterSkipAffectAppRevTrait(appRevision.Spec.Application.Spec, appRevision.Spec.TraitDefinitions))
	if err != nil {
		return "", err
	}
	for key, wd := range appRevision.Spec.WorkloadDefinitions {
		hash, err := utils.ComputeSpecHash(&wd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.WorkloadDefinitionHash[key] = hash
	}
	for key, cd := range appRevision.Spec.ComponentDefinitions {
		hash, err := utils.ComputeSpecHash(&cd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.ComponentDefinitionHash[key] = hash
	}
	for key, td := range appRevision.Spec.TraitDefinitions {
		hash, err := utils.ComputeSpecHash(&td.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.TraitDefinitionHash[key] = hash
	}
	for key, sd := range appRevision.Spec.ScopeDefinitions {
		hash, err := utils.ComputeSpecHash(&sd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHash.ScopeDefinitionHash[key] = hash
	}
	// compatible for old mode without any policy or workflow
	if len(appRevision.Spec.PolicyDefinitions) == 0 && len(appRevision.Spec.WorkflowStepDefinitions) == 0 {
		// compute the hash of the entire structure
		return utils.ComputeSpecHash(&appRevisionHash)
	}

	// Calculate Hash for New Mode with workflow and policy
	type AppRevisionHashWorkflow struct {
		ApplicationSpecHash        string
		WorkloadDefinitionHash     map[string]string
		ComponentDefinitionHash    map[string]string
		TraitDefinitionHash        map[string]string
		ScopeDefinitionHash        map[string]string
		PolicyDefinitionHash       map[string]string
		WorkflowStepDefinitionHash map[string]string
	}
	appRevisionHashWorkflow := AppRevisionHashWorkflow{
		ApplicationSpecHash:        appRevisionHash.ApplicationSpecHash,
		WorkloadDefinitionHash:     appRevisionHash.WorkloadDefinitionHash,
		ComponentDefinitionHash:    appRevisionHash.ComponentDefinitionHash,
		TraitDefinitionHash:        appRevisionHash.TraitDefinitionHash,
		ScopeDefinitionHash:        appRevisionHash.ScopeDefinitionHash,
		PolicyDefinitionHash:       make(map[string]string),
		WorkflowStepDefinitionHash: make(map[string]string),
	}
	for key, pd := range appRevision.Spec.PolicyDefinitions {
		hash, err := utils.ComputeSpecHash(&pd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHashWorkflow.PolicyDefinitionHash[key] = hash
	}
	for key, wd := range appRevision.Spec.WorkflowStepDefinitions {
		hash, err := utils.ComputeSpecHash(&wd.Spec)
		if err != nil {
			return "", err
		}
		appRevisionHashWorkflow.WorkflowStepDefinitionHash[key] = hash
	}
	return utils.ComputeSpecHash(&appRevisionHashWorkflow)
}

// currentAppRevIsNew check application revision already exist or not
func (h *AppHandler) currentAppRevIsNew(ctx context.Context) (bool, bool, error) {
	// the last revision doesn't exist.
	if h.app.Status.LatestRevision == nil {
		return true, true, nil
	}

	// diff the latest revision first
	if h.app.Status.LatestRevision.RevisionHash == h.currentRevHash && DeepEqualRevision(h.latestAppRev, h.currentAppRev) {
		h.currentAppRev = h.latestAppRev.DeepCopy()
		return false, false, nil
	}

	// list revision histories
	revisionList := &v1beta1.ApplicationRevisionList{}
	listOpts := []client.ListOption{client.MatchingLabels{
		oam.LabelAppName: h.app.Name,
	}, client.InNamespace(h.app.Namespace)}
	if err := h.r.Client.List(ctx, revisionList, listOpts...); err != nil {
		klog.ErrorS(err, "Failed to list app revision", "appName", h.app.Name)
		return false, false, errors.Wrap(err, "failed to list app revision")
	}

	for i := range revisionList.Items {
		if revisionList.Items[i].GetLabels()[oam.LabelAppRevisionHash] == h.currentRevHash && DeepEqualRevision(&revisionList.Items[i], h.currentAppRev) {
			// we set currentAppRev to existRevision
			h.currentAppRev = revisionList.Items[i].DeepCopy()
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
	if len(old.Spec.TraitDefinitions) != len(new.Spec.TraitDefinitions) {
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
	for key, td := range new.Spec.TraitDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.TraitDefinitions[key].Spec, td.Spec) {
			return false
		}
	}
	for key, sd := range new.Spec.ScopeDefinitions {
		if !apiequality.Semantic.DeepEqual(old.Spec.ScopeDefinitions[key].Spec, sd.Spec) {
			return false
		}
	}
	return apiequality.Semantic.DeepEqual(filterSkipAffectAppRevTrait(old.Spec.Application.Spec, old.Spec.TraitDefinitions),
		filterSkipAffectAppRevTrait(new.Spec.Application.Spec, new.Spec.TraitDefinitions))
}

// HandleComponentsRevision manages Component revisions
// 1. if update component create a new component Revision
// 2. check all componentTrait  rely on componentRevName, if yes fill it
func (h *AppHandler) HandleComponentsRevision(ctx context.Context, compManifests []*types.ComponentManifest) error {
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

	if err := h.r.Client.Get(ctx, client.ObjectKey{Namespace: h.getComponentRevisionNamespace(ctx), Name: revisionName}, cr); err != nil {
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
		oam.LabelControllerRevisionComponent: comp.Name,
	}, client.InNamespace(h.getComponentRevisionNamespace(ctx))}
	if err := h.r.List(ctx, crList, listOpts...); err != nil {
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
				oam.LabelAppComponent:                cm.Name,
				oam.LabelAppCluster:                  multicluster.ClusterNameInContext(ctx),
				oam.LabelAppEnv:                      envbinding.EnvNameInContext(ctx),
				oam.LabelControllerRevisionComponent: cm.Name,
				oam.LabelComponentRevisionHash:       cm.RevisionHash,
			},
		},
		Revision: int64(revision),
		Data:     *util.Object2RawExtension(comp),
	}
	common.NewOAMObjectReferenceFromObject(cm.StandardWorkload).AddLabelsToObject(cr)
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
	appRev := h.currentAppRev
	appRev.Namespace = h.app.Namespace
	appRev.SetGroupVersionKind(v1beta1.ApplicationRevisionGroupVersionKind)
	// pass application's annotations & labels to app revision
	appRev.SetAnnotations(h.app.GetAnnotations())
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
		Controller: pointer.BoolPtr(true),
	}})
	// In this stage, the configmap is empty and not generated.
	appRev.Spec.ResourcesConfigMap.Name = appRev.Name

	gotAppRev := &v1beta1.ApplicationRevision{}
	if err := h.r.Get(ctx, client.ObjectKey{Name: appRev.Name, Namespace: appRev.Namespace}, gotAppRev); err != nil {
		if apierrors.IsNotFound(err) {
			return h.r.Create(ctx, appRev)
		}
		return err
	}
	appRev.ResourceVersion = gotAppRev.ResourceVersion
	return h.r.Update(ctx, appRev)
}

// UpdateAppLatestRevisionStatus only call to update app's latest revision status after applying manifests successfully
// otherwise it will override previous revision which is used during applying to do GC jobs
func (h *AppHandler) UpdateAppLatestRevisionStatus(ctx context.Context) error {
	if !h.isNewRevision {
		// skip update if app revision is not changed
		return nil
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
	listOpts := []client.ListOption{
		client.InNamespace(h.app.Namespace),
		client.MatchingLabels{oam.LabelAppName: h.app.Name},
	}
	appRevisionList := new(v1beta1.ApplicationRevisionList)
	// controller-runtime will cache all appRevision by default, there is no need to watch or own appRevision in manager
	if err := h.r.List(ctx, appRevisionList, listOpts...); err != nil {
		return err
	}
	appRevisionInUse := gatherUsingAppRevision(h)
	needKill := len(appRevisionList.Items) - h.r.appRevisionLimit - len(appRevisionInUse)
	if needKill <= 0 {
		return nil
	}
	klog.InfoS("Going to garbage collect app revisions", "limit", h.r.appRevisionLimit,
		"total", len(appRevisionList.Items), "using", len(appRevisionInUse), "kill", needKill)
	sortedRevision := appRevisionList.Items
	sort.Sort(historiesByRevision(sortedRevision))

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
	if strings.Contains(str, model.ComponentRevisionPlaceHolder) {
		newStr := strings.ReplaceAll(str, model.ComponentRevisionPlaceHolder, compRevName)
		if err := json.Unmarshal([]byte(newStr), u); err != nil {
			return err
		}
	}
	return nil
}

// before computing hash or deepEqual, filterSkipAffectAppRevTrait filter can remove `SkipAffectAppRevTrait` trait from appSpec
func filterSkipAffectAppRevTrait(appSpec v1beta1.ApplicationSpec, tds map[string]v1beta1.TraitDefinition) v1beta1.ApplicationSpec {
	// deepCopy avoid modify origin appSpec
	res := appSpec.DeepCopy()
	for index, comp := range res.Components {
		i := 0
		for _, trait := range comp.Traits {
			if !tds[trait.Type].Spec.SkipRevisionAffect {
				comp.Traits[i] = trait
				i++
			}
		}
		res.Components[index].Traits = res.Components[index].Traits[:i]
	}
	return *res
}

type historiesByRevision []v1beta1.ApplicationRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	// the appRevision is generated by vela,  the error always is nil, so ignore it
	ir, _ := util.ExtractRevisionNum(h[i].Name, "-")
	ij, _ := util.ExtractRevisionNum(h[j].Name, "-")
	return ir < ij
}

func cleanUpWorkflowComponentRevision(ctx context.Context, h *AppHandler) error {
	// collect component revision in use
	compRevisionInUse := map[string]map[string]struct{}{}
	for _, resource := range h.app.Status.AppliedResources {
		compName := resource.Name
		ns := resource.Namespace
		r := &unstructured.Unstructured{}
		r.GetObjectKind().SetGroupVersionKind(resource.GroupVersionKind())
		_ctx := multicluster.ContextWithClusterName(ctx, resource.Cluster)
		err := h.r.Get(_ctx, ktypes.NamespacedName{Name: compName, Namespace: ns}, r)
		notFound := apierrors.IsNotFound(err)
		if err != nil && !notFound {
			return err
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
			oam.LabelControllerRevisionComponent: curComp.Name,
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
