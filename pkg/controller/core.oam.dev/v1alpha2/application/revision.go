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
	"bytes"
	"context"
	"reflect"
	"sort"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/dispatch"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// ConfigMapKeyResources is the key in ConfigMap Data field for containing data of resources
	ConfigMapKeyResources = "resources"
)

func (h *AppHandler) createResourcesConfigMap(ctx context.Context,
	appRev *v1beta1.ApplicationRevision,
	comps []*types.ComponentManifest,
	policies []*unstructured.Unstructured) error {

	buf := &bytes.Buffer{}
	for _, c := range comps {
		if c.InsertConfigNotReady {
			continue
		}
		r := c.StandardWorkload.DeepCopy()
		r.SetName(c.Name)
		r.SetNamespace(appRev.Namespace)
		buf.Write(util.MustJSONMarshal(r))
	}
	for _, c := range comps {
		if c.InsertConfigNotReady {
			continue
		}
		for _, tr := range c.Traits {
			r := tr.DeepCopy()
			r.SetName(c.Name)
			r.SetNamespace(appRev.Namespace)
			buf.Write(util.MustJSONMarshal(r))
		}
	}
	for _, policy := range policies {
		buf.Write(util.MustJSONMarshal(policy))
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
			ConfigMapKeyResources: buf.String(),
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
	af.RevisionName = h.currentAppRev.Name
	return nil
}

// gatherRevisionSpec will gather all revision spec withouth metadata and rendered result.
// the gathered Revision spec will be enough to calculate the hash and compare with the old revision
func (h *AppHandler) gatherRevisionSpec(af *appfile.Appfile) (*v1beta1.ApplicationRevision, string, error) {
	copiedApp := h.app.DeepCopy()
	// We better to remove all object status in the appRevision
	copiedApp.Status = common.AppStatus{}
	// AppRevision shouldn't contain RolloutPlan
	copiedApp.Spec.RolloutPlan = nil
	appRev := &v1beta1.ApplicationRevision{
		Spec: v1beta1.ApplicationRevisionSpec{
			Application:          *copiedApp,
			ComponentDefinitions: make(map[string]v1beta1.ComponentDefinition),
			WorkloadDefinitions:  make(map[string]v1beta1.WorkloadDefinition),
			TraitDefinitions:     make(map[string]v1beta1.TraitDefinition),
			ScopeDefinitions:     make(map[string]v1beta1.ScopeDefinition),
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
		// TODO(wonderflow): take scope into the revision
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
	appRevisionHash.ApplicationSpecHash, err = utils.ComputeSpecHash(&appRevision.Spec.Application.Spec)
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
	// compute the hash of the entire structure
	return utils.ComputeSpecHash(&appRevisionHash)
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
	return apiequality.Semantic.DeepEqual(&old.Spec.Application.Spec, &new.Spec.Application.Spec)
}

// HandleComponentsRevision manages Component revisions
func (h *AppHandler) HandleComponentsRevision(ctx context.Context, compManifests []*types.ComponentManifest) error {
	for _, cm := range compManifests {
		if cm.InsertConfigNotReady {
			continue
		}
		hash, err := computeComponentRevisionHash(cm)
		if err != nil {
			return err
		}
		cm.RevisionHash = hash

		crList := &appsv1.ControllerRevisionList{}
		listOpts := []client.ListOption{client.MatchingLabels{
			oam.LabelControllerRevisionComponent: cm.Name,
		}, client.InNamespace(h.app.Namespace)}
		if err := h.r.List(ctx, crList, listOpts...); err != nil {
			return err
		}

		var maxRevisionNum int64
		needNewRevision := true
		for _, existingCR := range crList.Items {
			if existingCR.Revision > maxRevisionNum {
				maxRevisionNum = existingCR.Revision
			}
			if existingCR.GetLabels()[oam.LabelComponentRevisionHash] == cm.RevisionHash {
				existingComp, err := util.RawExtension2Component(existingCR.Data)
				if err != nil {
					return err
				}
				currentComp := componentManifest2Component(cm)
				// further check whether it's truly identical, even hash value is equal
				if reflect.DeepEqual(existingComp, currentComp) {
					cm.RevisionName = existingCR.GetName()
					// found identical revision already exisits
					// skip creating new one
					needNewRevision = false
					break
				}
			}
		}
		if needNewRevision {
			cm.RevisionName = utils.ConstructRevisionName(cm.Name, maxRevisionNum+1)
			if err := h.createControllerRevision(ctx, cm); err != nil {
				return err
			}
		}
	}
	return nil
}

func computeComponentRevisionHash(comp *types.ComponentManifest) (string, error) {
	compRevisionHash := struct {
		WorkloadHash          string
		PackagedResourcesHash []string
	}{}
	wl := comp.StandardWorkload.DeepCopy()
	if wl != nil {
		// remove workload's app revision label before computing component hash
		// otherwise different app revision will always have different revision component
		util.RemoveLabels(wl, []string{oam.LabelAppRevision})
		hash, err := utils.ComputeSpecHash(wl)
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
	comp := componentManifest2Component(cm)
	revision, _ := utils.ExtractRevision(cm.RevisionName)
	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.RevisionName,
			Namespace: h.app.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
					Name:       h.app.Name,
					UID:        h.app.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
			Labels: map[string]string{
				oam.LabelControllerRevisionComponent: cm.Name,
				oam.LabelComponentRevisionHash:       cm.RevisionHash,
			},
		},
		Revision: int64(revision),
		Data:     util.Object2RawExtension(comp),
	}
	return h.r.Create(ctx, cr)
}

func componentManifest2Component(cm *types.ComponentManifest) *v1alpha2.Component {
	component := &v1alpha2.Component{}
	component.SetGroupVersionKind(v1alpha2.ComponentGroupVersionKind)
	component.SetName(cm.Name)
	wl := cm.StandardWorkload.DeepCopy()
	if wl != nil {
		util.RemoveLabels(wl, []string{oam.LabelAppRevision})
	}
	component.Spec.Workload = util.Object2RawExtension(wl)
	if len(cm.PackagedWorkloadResources) > 0 {
		helm := &common.Helm{}
		for _, helmResource := range cm.PackagedWorkloadResources {
			if helmResource.GetKind() == helmapi.HelmReleaseGVK.Kind {
				helm.Release = util.Object2RawExtension(helmResource)
			}
			if helmResource.GetKind() == helmapi.HelmRepositoryGVK.Kind {
				helm.Repository = util.Object2RawExtension(helmResource)
			}
		}
		component.Spec.Helm = helm
	}
	return component
}

// FinalizeAndApplyAppRevision finalise AppRevision object and apply it
func (h *AppHandler) FinalizeAndApplyAppRevision(ctx context.Context, comps []*types.ComponentManifest) error {
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
	appRev.Spec.ResourcesConfigMap.Name = appRev.Name
	appRev.Spec.ApplicationConfiguration, appRev.Spec.Components = componentManifests2AppConfig(comps)

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

// helper function to convert a slice of ComponentManifest to AppConfig & Components
func componentManifests2AppConfig(cms []*types.ComponentManifest) (runtime.RawExtension, []common.RawComponent) {
	ac := v1alpha2.ApplicationConfiguration{}
	ac.SetGroupVersionKind(v1alpha2.ApplicationConfigurationGroupVersionKind)
	ac.Spec.Components = make([]v1alpha2.ApplicationConfigurationComponent, len(cms))
	comps := make([]common.RawComponent, len(cms))
	for i, cm := range cms {
		acc := v1alpha2.ApplicationConfigurationComponent{}
		acc.ComponentName = cm.Name
		comp := &v1alpha2.Component{}
		comp.SetGroupVersionKind(v1alpha2.ComponentGroupVersionKind)
		comp.SetName(cm.Name)

		if cm.InsertConfigNotReady {
			// -- represent the component is not ready at all
			acc.RevisionName = "--"

			comps[i] = common.RawComponent{Raw: util.Object2RawExtension(comp)}
			ac.Spec.Components[i] = acc
			continue
		}

		acc.RevisionName = cm.RevisionName
		acc.Traits = make([]v1alpha2.ComponentTrait, len(cm.Traits))
		for j, t := range cm.Traits {
			acc.Traits[j] = v1alpha2.ComponentTrait{
				Trait: util.Object2RawExtension(t),
			}
		}
		acc.Scopes = make([]v1alpha2.ComponentScope, len(cm.Scopes))
		for x, s := range cm.Scopes {
			acc.Scopes[x] = v1alpha2.ComponentScope{
				ScopeReference: runtimev1alpha1.TypedReference{
					APIVersion: s.APIVersion,
					Kind:       s.Kind,
					Name:       s.Name,
				},
			}
		}
		acc.WorkloadManagedByTrait = cm.WorkloadManagedByTrait
		// this label is very important for handling component revision
		util.AddLabels(comp, map[string]string{
			oam.LabelComponentRevisionHash: cm.RevisionHash,
		})
		comp.Spec.Workload = util.Object2RawExtension(cm.StandardWorkload)
		if len(cm.PackagedWorkloadResources) > 0 {
			helm := &common.Helm{}
			for _, helmResource := range cm.PackagedWorkloadResources {
				if helmResource.GetKind() == helmapi.HelmReleaseGVK.Kind {
					helm.Release = util.Object2RawExtension(helmResource)
				}
				if helmResource.GetKind() == helmapi.HelmRepositoryGVK.Kind {
					helm.Repository = util.Object2RawExtension(helmResource)
				}
			}
			comp.Spec.Helm = helm
		}
		comps[i] = common.RawComponent{Raw: util.Object2RawExtension(comp)}
		ac.Spec.Components[i] = acc
	}
	acRaw := util.Object2RawExtension(ac)
	return acRaw, comps
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
	if err := h.r.patchStatus(ctx, h.app); err != nil {
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
	appRevisionInUse, err := gatherUsingAppRevision(ctx, h)
	if err != nil {
		return err
	}
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

// gatherUsingAppRevision get all using appRevisions include app's status pointing to and appContext point to
func gatherUsingAppRevision(ctx context.Context, h *AppHandler) (map[string]bool, error) {
	ns := h.app.Namespace
	listOpts := []client.ListOption{
		client.MatchingLabels{
			oam.LabelAppName:      h.app.Name,
			oam.LabelAppNamespace: ns,
		}}
	usingRevision := map[string]bool{}
	if h.app.Status.LatestRevision != nil && len(h.app.Status.LatestRevision.Name) != 0 {
		usingRevision[h.app.Status.LatestRevision.Name] = true
	}
	rtList := &v1beta1.ResourceTrackerList{}
	if err := h.r.List(ctx, rtList, listOpts...); err != nil {
		return nil, err
	}
	for _, rt := range rtList.Items {
		appRev := dispatch.ExtractAppRevisionName(rt.Name, ns)
		usingRevision[appRev] = true
	}
	appDeployUsingRevision, err := utils.CheckAppDeploymentUsingAppRevision(ctx, h.r, h.app.Namespace, h.app.Name)
	if err != nil {
		return usingRevision, err
	}
	for _, revName := range appDeployUsingRevision {
		usingRevision[revName] = true
	}
	return usingRevision, nil
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

func cleanUpComponentRevision(ctx context.Context, h *AppHandler) error {
	appRevInUse, err := gatherUsingAppRevision(ctx, h)
	if err != nil {
		return err
	}
	// collect component revision in use
	compRevisionInUse := map[string]map[string]struct{}{}
	for appRevName := range appRevInUse {
		appRev := &v1beta1.ApplicationRevision{}
		if err := h.r.Get(ctx, client.ObjectKey{Name: appRevName, Namespace: h.app.Namespace}, appRev); err != nil {
			return err
		}
		comps, err := util.AppConfig2ComponentManifests(appRev.Spec.ApplicationConfiguration, appRev.Spec.Components)
		if err != nil {
			return err
		}
		for _, comp := range comps {
			if compRevisionInUse[comp.Name] == nil {
				compRevisionInUse[comp.Name] = map[string]struct{}{}
			}
			compRevisionInUse[comp.Name][comp.RevisionName] = struct{}{}
		}
	}

	comps, err := util.AppConfig2ComponentManifests(h.currentAppRev.Spec.ApplicationConfiguration,
		h.currentAppRev.Spec.Components)
	if err != nil {
		return err
	}
	for _, curComp := range comps {
		crList := &appsv1.ControllerRevisionList{}
		listOpts := []client.ListOption{client.MatchingLabels{
			oam.LabelControllerRevisionComponent: curComp.Name,
		}, client.InNamespace(h.app.Namespace)}
		if err := h.r.List(ctx, crList, listOpts...); err != nil {
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
			if err := h.r.Delete(ctx, rev.DeepCopy()); err != nil && !apierrors.IsNotFound(err) {
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
