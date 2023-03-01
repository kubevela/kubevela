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

package resourcekeeper

import (
	"context"
	"encoding/json"
	"math/rand"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	version2 "github.com/oam-dev/kubevela/version"
)

var (
	// MarkWithProbability optimize ResourceTracker gc for legacy resource by reducing the frequency of outdated rt check
	MarkWithProbability = 0.1
)

// GCOption option for gc
type GCOption interface {
	ApplyToGCConfig(*gcConfig)
}

type gcConfig struct {
	passive bool

	disableMark                bool
	disableSweep               bool
	disableFinalize            bool
	disableComponentRevisionGC bool
	disableLegacyGC            bool

	order v1alpha1.GarbageCollectOrder
}

func newGCConfig(options ...GCOption) *gcConfig {
	cfg := &gcConfig{}
	for _, option := range options {
		option.ApplyToGCConfig(cfg)
	}
	return cfg
}

// GarbageCollect recycle resources and handle finalizers for resourcetracker
// Application Resource Garbage Collection follows three stages
//
// 1. Mark Stage
// Controller will find all resourcetrackers for the target application and decide which resourcetrackers should be
// deleted. Decision rules including:
//
//	a. rootRT and currentRT will be marked as deleted only when application is marked as deleted (DeleteTimestamp is
//	   not nil).
//	b. historyRTs will be marked as deleted if at least one of the below conditions met
//	   i.  GarbageCollectionMode is not set to `passive`
//	   ii. All managed resources are RECYCLED. (RECYCLED means resource does not exist or managed by latest
//	       resourcetrackers)
//
// NOTE: Mark Stage will always work for each application reconcile, not matter whether workflow is ended
//
// 2. Sweep Stage
// Controller will check all resourcetrackers marked to be deleted. If all managed resources are recycled, finalizer in
// resourcetracker will be removed.
//
// 3. Finalize Stage
// Controller will finalize all resourcetrackers marked to be deleted. All managed resources are recycled.
//
// NOTE: Mark Stage will only work when Workflow succeeds. Check/Finalize Stage will always work.
//
//	For one single application, the deletion will follow Mark -> Finalize -> Sweep
func (h *resourceKeeper) GarbageCollect(ctx context.Context, options ...GCOption) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	if h.garbageCollectPolicy != nil {
		if h.garbageCollectPolicy.KeepLegacyResource {
			options = append(options, PassiveGCOption{})
		}
		switch h.garbageCollectPolicy.Order {
		case v1alpha1.OrderDependency:
			options = append(options, DependencyGCOption{})
		default:
		}
	}
	cfg := newGCConfig(options...)
	return h.garbageCollect(ctx, cfg)
}

func (h *resourceKeeper) garbageCollect(ctx context.Context, cfg *gcConfig) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	gc := gcHandler{resourceKeeper: h, cfg: cfg}
	gc.Init()
	// Mark Stage
	if !cfg.disableMark {
		if err = gc.Mark(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to mark inactive resourcetrackers")
		}
	}
	// Sweep Stage
	if !cfg.disableSweep {
		if finished, waiting, err = gc.Sweep(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to sweep resourcetrackers to be deleted")
		}
	}
	// Finalize Stage
	if !cfg.disableFinalize && !finished {
		if err = gc.Finalize(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to finalize resourcetrackers to be deleted")
		}
	}
	// Garbage Collect Component Revision in unused components
	if !cfg.disableComponentRevisionGC {
		if err = gc.GarbageCollectComponentRevisionResourceTracker(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to garbage collect component revisions in unused components")
		}
	}
	// Garbage Collect Legacy ResourceTrackers
	if !cfg.disableLegacyGC {
		if err = gc.GarbageCollectLegacyResourceTrackers(ctx); err != nil {
			return false, waiting, errors.Wrapf(err, "failed to garbage collect legacy resource trackers")
		}
	}
	return finished, waiting, nil
}

// gcHandler gc detail implementations
type gcHandler struct {
	*resourceKeeper
	cfg *gcConfig
}

func (h *gcHandler) monitor(stage string) func() {
	begin := time.Now()
	return func() {
		v := time.Since(begin).Seconds()
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-rt." + stage).Observe(v)
	}
}

func (h *gcHandler) regularizeResourceTracker(rts ...*v1beta1.ResourceTracker) {
	for _, rt := range rts {
		if rt == nil {
			continue
		}
		for i, mr := range rt.Spec.ManagedResources {
			if ok, err := utils.IsClusterScope(mr.GroupVersionKind(), h.Client.RESTMapper()); err == nil && ok {
				rt.Spec.ManagedResources[i].Namespace = ""
			}
		}
	}
}

func (h *gcHandler) Init() {
	cb := h.monitor("init")
	defer cb()
	rts := append(h._historyRTs, h._currentRT, h._rootRT) // nolint
	h.regularizeResourceTracker(rts...)
	h.cache.registerResourceTrackers(rts...)
}

func (h *gcHandler) scan(ctx context.Context) (inactiveRTs []*v1beta1.ResourceTracker) {
	if h.app.GetDeletionTimestamp() != nil {
		inactiveRTs = append(inactiveRTs, h._historyRTs...)
		inactiveRTs = append(inactiveRTs, h._currentRT, h._rootRT, h._crRT)
	} else {
		if h.cfg.passive {
			inactiveRTs = []*v1beta1.ResourceTracker{}
			if rand.Float64() > MarkWithProbability { //nolint
				return inactiveRTs
			}
			for _, rt := range h._historyRTs {
				if rt != nil {
					inactive := true
					for _, mr := range rt.Spec.ManagedResources {
						entry := h.cache.get(auth.ContextWithUserInfo(ctx, h.app), mr)
						if entry.err == nil && (entry.gcExecutorRT != rt || !entry.exists) {
							continue
						}
						inactive = false
					}
					if inactive {
						inactiveRTs = append(inactiveRTs, rt)
					}
				}
			}
		} else {
			inactiveRTs = h._historyRTs
		}
	}
	return inactiveRTs
}

func (h *gcHandler) Mark(ctx context.Context) error {
	cb := h.monitor("mark")
	defer cb()
	inactiveRTs := h.scan(ctx)
	for _, rt := range inactiveRTs {
		if rt != nil && rt.GetDeletionTimestamp() == nil {
			if err := h.Client.Delete(ctx, rt); err != nil && !kerrors.IsNotFound(err) {
				return err
			}
			_rt := &v1beta1.ResourceTracker{}
			if err := h.Client.Get(ctx, client.ObjectKeyFromObject(rt), _rt); err != nil {
				if !kerrors.IsNotFound(err) {
					return err
				}
			} else {
				_rt.DeepCopyInto(rt)
			}
		}
	}
	return nil
}

// checkAndRemoveResourceTrackerFinalizer return (all resource recycled, error)
func (h *gcHandler) checkAndRemoveResourceTrackerFinalizer(ctx context.Context, rt *v1beta1.ResourceTracker) (bool, v1beta1.ManagedResource, error) {
	for _, mr := range rt.Spec.ManagedResources {
		entry := h.cache.get(auth.ContextWithUserInfo(ctx, h.app), mr)
		if entry.err != nil {
			return false, entry.mr, entry.err
		}
		if entry.exists && entry.gcExecutorRT == rt {
			return false, entry.mr, nil
		}
	}
	meta.RemoveFinalizer(rt, resourcetracker.Finalizer)
	return true, v1beta1.ManagedResource{}, h.Client.Update(ctx, rt)
}

func (h *gcHandler) Sweep(ctx context.Context) (finished bool, waiting []v1beta1.ManagedResource, err error) {
	cb := h.monitor("sweep")
	defer cb()
	finished = true
	for _, rt := range append(h._historyRTs, h._currentRT, h._rootRT) {
		if rt != nil && rt.GetDeletionTimestamp() != nil {
			_finished, mr, err := h.checkAndRemoveResourceTrackerFinalizer(ctx, rt)
			if err != nil {
				return false, waiting, err
			}
			if !_finished {
				finished = false
				waiting = append(waiting, mr)
			}
		}
	}
	return finished, waiting, nil
}

func (h *gcHandler) recycleResourceTracker(ctx context.Context, rt *v1beta1.ResourceTracker) error {
	ctx = auth.ContextWithUserInfo(ctx, h.app)
	switch h.cfg.order {
	case v1alpha1.OrderDependency:
		for _, mr := range rt.Spec.ManagedResources {
			if err := h.deleteIndependentComponent(ctx, mr, rt); err != nil {
				return err
			}
		}
		return nil
	default:
	}
	for _, mr := range rt.Spec.ManagedResources {
		if err := h.deleteManagedResource(ctx, mr, rt); err != nil {
			return err
		}
	}
	return nil
}

func (h *gcHandler) deleteIndependentComponent(ctx context.Context, mr v1beta1.ManagedResource, rt *v1beta1.ResourceTracker) error {
	dependent := h.checkDependentComponent(mr)
	if len(dependent) == 0 {
		if err := h.deleteManagedResource(ctx, mr, rt); err != nil {
			return err
		}
	} else {
		dependentClear := true
		for _, mr := range rt.Spec.ManagedResources {
			if utils.StringsContain(dependent, mr.Component) {
				entry := h.cache.get(ctx, mr)
				if entry.gcExecutorRT != rt {
					continue
				}
				if entry.err != nil {
					continue
				}
				if entry.exists {
					dependentClear = false
					break
				}
			}
		}
		if dependentClear {
			if err := h.deleteManagedResource(ctx, mr, rt); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdateSharedManagedResourceOwner update owner & sharer labels for managed resource
func UpdateSharedManagedResourceOwner(ctx context.Context, cli client.Client, manifest *unstructured.Unstructured, sharedBy string) error {
	parts := strings.Split(apply.FirstSharer(sharedBy), "/")
	appName, appNs := "", metav1.NamespaceDefault
	if len(parts) == 1 {
		appName = parts[0]
	} else if len(parts) == 2 {
		appName, appNs = parts[1], parts[0]
	}
	util.AddAnnotations(manifest, map[string]string{oam.AnnotationAppSharedBy: sharedBy})
	util.AddLabels(manifest, map[string]string{
		oam.LabelAppName:      appName,
		oam.LabelAppNamespace: appNs,
	})
	return cli.Update(ctx, manifest)
}

func (h *gcHandler) deleteManagedResource(ctx context.Context, mr v1beta1.ManagedResource, rt *v1beta1.ResourceTracker) error {
	entry := h.cache.get(ctx, mr)
	if entry.gcExecutorRT != rt {
		return nil
	}
	if entry.err != nil {
		return entry.err
	}
	if entry.exists {
		return DeleteManagedResourceInApplication(ctx, h.Client, mr, entry.obj, h.app)
	}
	return nil
}

// DeleteManagedResourceInApplication delete managed resource in application
func DeleteManagedResourceInApplication(ctx context.Context, cli client.Client, mr v1beta1.ManagedResource, obj *unstructured.Unstructured, app *v1beta1.Application) error {
	_ctx := multicluster.ContextWithClusterName(ctx, mr.Cluster)
	if annotations := obj.GetAnnotations(); annotations != nil && annotations[oam.AnnotationAppSharedBy] != "" {
		sharedBy := apply.RemoveSharer(annotations[oam.AnnotationAppSharedBy], app)
		if sharedBy != "" {
			if err := UpdateSharedManagedResourceOwner(_ctx, cli, obj, sharedBy); err != nil {
				return errors.Wrapf(err, "failed to remove sharer from resource %s", mr.ResourceKey())
			}
			return nil
		}
		util.RemoveAnnotations(obj, []string{oam.AnnotationAppSharedBy})
	}
	if mr.SkipGC || hasOrphanFinalizer(app) {
		if labels := obj.GetLabels(); labels != nil {
			delete(labels, oam.LabelAppName)
			delete(labels, oam.LabelAppNamespace)
			obj.SetLabels(labels)
		}
		return errors.Wrapf(cli.Update(_ctx, obj), "failed to remove owner labels for resource while skipping gc")
	}
	if err := cli.Delete(_ctx, obj); err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete resource %s", mr.ResourceKey())
	}
	return nil
}

func (h *gcHandler) checkDependentComponent(mr v1beta1.ManagedResource) []string {
	dependent := make([]string, 0)
	outputs := make([]string, 0)
	for _, comp := range h.app.Spec.Components {
		if comp.Name == mr.Component {
			for _, output := range comp.Outputs {
				outputs = append(outputs, output.Name)
			}
		} else {
			for _, dependsOn := range comp.DependsOn {
				if dependsOn == mr.Component {
					dependent = append(dependent, comp.Name)
					break
				}
			}
		}
	}
	for _, comp := range h.app.Spec.Components {
		for _, input := range comp.Inputs {
			if utils.StringsContain(outputs, input.From) {
				dependent = append(dependent, comp.Name)
			}
		}
	}
	return dependent
}

func (h *gcHandler) Finalize(ctx context.Context) error {
	cb := h.monitor("finalize")
	defer cb()
	for _, rt := range append(h._historyRTs, h._currentRT, h._rootRT) {
		if rt != nil && rt.GetDeletionTimestamp() != nil && meta.FinalizerExists(rt, resourcetracker.Finalizer) {
			if err := h.recycleResourceTracker(ctx, rt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *gcHandler) GarbageCollectComponentRevisionResourceTracker(ctx context.Context) error {
	cb := h.monitor("comp-rev")
	defer cb()
	if h._crRT == nil {
		return nil
	}
	inUseComponents := map[string]bool{}
	for _, entry := range h.cache.m.Data() {
		for _, rt := range entry.usedBy {
			if rt.GetDeletionTimestamp() == nil || len(rt.GetFinalizers()) != 0 {
				inUseComponents[entry.mr.ComponentKey()] = true
			}
		}
	}
	var managedResources []v1beta1.ManagedResource
	for _, cr := range h._crRT.Spec.ManagedResources { // legacy code for rollout-plan
		_ctx := multicluster.ContextWithClusterName(ctx, cr.Cluster)
		_ctx = auth.ContextWithUserInfo(_ctx, h.app)
		if _, exists := inUseComponents[cr.ComponentKey()]; !exists {
			_cr := &appsv1.ControllerRevision{}
			err := h.Client.Get(_ctx, cr.NamespacedName(), _cr)
			if err != nil && !multicluster.IsNotFoundOrClusterNotExists(err) {
				return errors.Wrapf(err, "failed to get component revision %s", cr.ResourceKey())
			}
			if err == nil {
				if err = h.Client.Delete(_ctx, _cr); err != nil && !kerrors.IsNotFound(err) {
					return errors.Wrapf(err, "failed to delete component revision %s", cr.ResourceKey())
				}
			}
		} else {
			managedResources = append(managedResources, cr)
		}
	}
	h._crRT.Spec.ManagedResources = managedResources
	if len(managedResources) == 0 && h._crRT.GetDeletionTimestamp() != nil {
		meta.RemoveFinalizer(h._crRT, resourcetracker.Finalizer)
	}
	if err := h.Client.Update(ctx, h._crRT); err != nil {
		return errors.Wrapf(err, "failed to update controllerrevision RT %s", h._crRT.Name)
	}
	return nil
}

const velaVersionNumberToUpgradeResourceTracker = "v1.2.0"

func (h *gcHandler) GarbageCollectLegacyResourceTrackers(ctx context.Context) error {
	// skip legacy gc if controller not enable this feature
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.LegacyResourceTrackerGC) {
		return nil
	}
	// skip legacy gc if application is not handled by new version rt
	if h.app.GetDeletionTimestamp() == nil && h.resourceKeeper._currentRT == nil {
		return nil
	}
	// check app version
	velaVersionToUpgradeResourceTracker, _ := version.NewVersion(velaVersionNumberToUpgradeResourceTracker)
	var currentVersionNumber string
	if annotations := h.app.GetAnnotations(); annotations != nil && annotations[oam.AnnotationKubeVelaVersion] != "" {
		currentVersionNumber = annotations[oam.AnnotationKubeVelaVersion]
	}
	currentVersion, err := version.NewVersion(currentVersionNumber)
	if err == nil && velaVersionToUpgradeResourceTracker.LessThanOrEqual(currentVersion) {
		return nil
	}
	// remove legacy ResourceTrackers
	clusters := map[string]bool{multicluster.ClusterLocalName: true}
	for _, rsc := range h.app.Status.AppliedResources {
		if rsc.Cluster != "" {
			clusters[rsc.Cluster] = true
		}
	}
	for _, policy := range h.app.Spec.Policies {
		if policy.Type == v1alpha1.EnvBindingPolicyType && policy.Properties != nil {
			spec := &v1alpha1.EnvBindingSpec{}
			if err = json.Unmarshal(policy.Properties.Raw, &spec); err == nil {
				for _, env := range spec.Envs {
					if env.Placement.ClusterSelector != nil && env.Placement.ClusterSelector.Name != "" {
						clusters[env.Placement.ClusterSelector.Name] = true
					}
				}
			}
		}
	}
	for cluster := range clusters {
		_ctx := multicluster.ContextWithClusterName(ctx, cluster)
		rts := &unstructured.UnstructuredList{}
		rts.SetGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("ResourceTrackerList"))
		if err = h.Client.List(_ctx, rts, client.MatchingLabels(map[string]string{
			oam.LabelAppName:      h.app.Name,
			oam.LabelAppNamespace: h.app.Namespace,
		})); err != nil {
			if strings.Contains(err.Error(), "could not find the requested resource") {
				continue
			}
			return errors.Wrapf(err, "failed to list resource trackers for app %s/%s in cluster %s", h.app.Namespace, h.app.Name, cluster)
		}
		for _, rt := range rts.Items {
			if s, exists, _ := unstructured.NestedString(rt.Object, "spec", "type"); !exists || s == "" {
				if err = h.Client.Delete(_ctx, rt.DeepCopy()); err != nil {
					return errors.Wrapf(err, "failed to delete legacy resource tracker %s for app %s/%s in cluster %s", rt.GetName(), h.app.Namespace, h.app.Name, cluster)
				}
			}
		}
	}
	// upgrade app version
	app := &v1beta1.Application{}
	if err = h.Client.Get(ctx, client.ObjectKeyFromObject(h.app), app); err != nil {
		return errors.Wrapf(err, "failed to get app %s/%s for upgrade version", h.app.Namespace, h.app.Name)
	}
	if _, err = version.NewVersion(version2.VelaVersion); err != nil {
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationKubeVelaVersion, velaVersionNumberToUpgradeResourceTracker)
	} else {
		metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationKubeVelaVersion, version2.VelaVersion)
	}
	if err = h.Client.Update(ctx, app); err != nil {
		return errors.Wrapf(err, "failed to upgrade app %s/%s", h.app.Namespace, h.app.Name)
	}
	h.app.ObjectMeta = app.ObjectMeta
	return nil
}
