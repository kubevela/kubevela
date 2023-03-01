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
	"sync"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
	terraforv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	terraforv1beta2 "github.com/oam-dev/terraform-controller/api/v1beta2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// AppHandler handles application reconcile
type AppHandler struct {
	r              *Reconciler
	app            *v1beta1.Application
	currentAppRev  *v1beta1.ApplicationRevision
	latestAppRev   *v1beta1.ApplicationRevision
	resourceKeeper resourcekeeper.ResourceKeeper

	isNewRevision  bool
	currentRevHash string

	services         []common.ApplicationComponentStatus
	appliedResources []common.ClusterObjectReference
	deletedResources []common.ClusterObjectReference
	parser           *appfile.Parser

	mu sync.Mutex
}

// NewAppHandler create new app handler
func NewAppHandler(ctx context.Context, r *Reconciler, app *v1beta1.Application, parser *appfile.Parser) (*AppHandler, error) {
	if ctx, ok := ctx.(monitorContext.Context); ok {
		subCtx := ctx.Fork("create-app-handler", monitorContext.DurationMetric(func(v float64) {
			metrics.AppReconcileStageDurationHistogram.WithLabelValues("create-app-handler").Observe(v)
		}))
		defer subCtx.Commit("finish create appHandler")
	}
	resourceHandler, err := resourcekeeper.NewResourceKeeper(ctx, r.Client, app)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create resourceKeeper")
	}
	return &AppHandler{
		r:              r,
		app:            app,
		resourceKeeper: resourceHandler,
		parser:         parser,
	}, nil
}

// Dispatch apply manifests into k8s.
func (h *AppHandler) Dispatch(ctx context.Context, cluster string, owner string, manifests ...*unstructured.Unstructured) error {
	manifests = multicluster.ResourcesWithClusterName(cluster, manifests...)
	if err := h.resourceKeeper.Dispatch(ctx, manifests, nil); err != nil {
		return err
	}
	for _, mf := range manifests {
		if mf == nil {
			continue
		}
		if oam.GetCluster(mf) != "" {
			cluster = oam.GetCluster(mf)
		}
		ref := common.ClusterObjectReference{
			Cluster: cluster,
			Creator: owner,
			ObjectReference: corev1.ObjectReference{
				Name:       mf.GetName(),
				Namespace:  mf.GetNamespace(),
				Kind:       mf.GetKind(),
				APIVersion: mf.GetAPIVersion(),
			},
		}
		h.addAppliedResource(false, ref)
	}
	return nil
}

// Delete delete manifests from k8s.
func (h *AppHandler) Delete(ctx context.Context, cluster string, owner string, manifest *unstructured.Unstructured) error {
	manifests := multicluster.ResourcesWithClusterName(cluster, manifest)
	if err := h.resourceKeeper.Delete(ctx, manifests); err != nil {
		return err
	}
	ref := common.ClusterObjectReference{
		Cluster: cluster,
		Creator: owner,
		ObjectReference: corev1.ObjectReference{
			Name:       manifest.GetName(),
			Namespace:  manifest.GetNamespace(),
			Kind:       manifest.GetKind(),
			APIVersion: manifest.GetAPIVersion(),
		},
	}
	h.deleteAppliedResource(ref)
	return nil
}

// addAppliedResource recorde applied resource.
// reconcile run at single threaded. So there is no need to consider to use locker.
func (h *AppHandler) addAppliedResource(previous bool, refs ...common.ClusterObjectReference) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ref := range refs {
		if previous {
			for i, deleted := range h.deletedResources {
				if deleted.Equal(ref) {
					h.deletedResources = removeResources(h.deletedResources, i)
					return
				}
			}
		}

		found := false
		for _, current := range h.appliedResources {
			if current.Equal(ref) {
				found = true
				break
			}
		}
		if !found {
			h.appliedResources = append(h.appliedResources, ref)
		}
	}
}

func (h *AppHandler) deleteAppliedResource(ref common.ClusterObjectReference) {
	delIndex := -1
	for i, current := range h.appliedResources {
		if current.Equal(ref) {
			delIndex = i
		}
	}
	if delIndex < 0 {
		isDeleted := false
		for _, deleted := range h.deletedResources {
			if deleted.Equal(ref) {
				isDeleted = true
				break
			}
		}
		if !isDeleted {
			h.deletedResources = append(h.deletedResources, ref)
		}
	} else {
		h.appliedResources = removeResources(h.appliedResources, delIndex)
	}

}

func removeResources(elements []common.ClusterObjectReference, index int) []common.ClusterObjectReference {
	elements[index] = elements[len(elements)-1]
	return elements[:len(elements)-1]
}

// getServiceStatus get specified component status
func (h *AppHandler) getServiceStatus(svc common.ApplicationComponentStatus) common.ApplicationComponentStatus {
	for i := range h.services {
		current := h.services[i]
		if current.Equal(svc) {
			return current
		}
	}
	return svc
}

// addServiceStatus recorde the whole component status.
// reconcile run at single threaded. So there is no need to consider to use locker.
func (h *AppHandler) addServiceStatus(cover bool, svcs ...common.ApplicationComponentStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, svc := range svcs {
		found := false
		for i := range h.services {
			current := h.services[i]
			if current.Equal(svc) {
				if cover {
					h.services[i] = svc
				}
				found = true
				break
			}
		}
		if !found {
			h.services = append(h.services, svc)
		}
	}
}

// ProduceArtifacts will produce Application artifacts that will be saved in configMap.
func (h *AppHandler) ProduceArtifacts(ctx context.Context, comps []*types.ComponentManifest, policies []*unstructured.Unstructured) error {
	return h.createResourcesConfigMap(ctx, h.currentAppRev, comps, policies)
}

// collectTraitHealthStatus collect trait health status
func (h *AppHandler) collectTraitHealthStatus(wl *appfile.Workload, tr *appfile.Trait, appRev *v1beta1.ApplicationRevision, overrideNamespace string) (common.ApplicationTraitStatus, []*unstructured.Unstructured, error) {
	defer func(clusterName string) {
		wl.Ctx.SetCtx(pkgmulticluster.WithCluster(wl.Ctx.GetCtx(), clusterName))
	}(multicluster.ClusterNameInContext(wl.Ctx.GetCtx()))

	var (
		pCtx        = wl.Ctx
		appName     = appRev.Spec.Application.Name
		traitStatus = common.ApplicationTraitStatus{
			Type:    tr.Name,
			Healthy: true,
		}
		traitOverrideNamespace = overrideNamespace
		err                    error
	)
	if tr.FullTemplate.TraitDefinition.Spec.ControlPlaneOnly {
		traitOverrideNamespace = appRev.GetNamespace()
		pCtx.SetCtx(pkgmulticluster.WithCluster(pCtx.GetCtx(), pkgmulticluster.Local))
	}
	_accessor := util.NewApplicationResourceNamespaceAccessor(h.app.Namespace, traitOverrideNamespace)
	templateContext, err := tr.GetTemplateContext(pCtx, h.r.Client, _accessor)
	if err != nil {
		return common.ApplicationTraitStatus{}, nil, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, get template context error", appName, wl.Name, tr.Name)
	}
	if ok, err := tr.EvalHealth(templateContext); !ok || err != nil {
		traitStatus.Healthy = false
	}
	traitStatus.Message, err = tr.EvalStatus(templateContext)
	if err != nil {
		return common.ApplicationTraitStatus{}, nil, errors.WithMessagef(err, "app=%s, comp=%s, trait=%s, evaluate status message error", appName, wl.Name, tr.Name)
	}
	return traitStatus, extractOutputs(templateContext), nil
}

// collectWorkloadHealthStatus collect workload health status
func (h *AppHandler) collectWorkloadHealthStatus(ctx context.Context, wl *appfile.Workload, appRev *v1beta1.ApplicationRevision, status *common.ApplicationComponentStatus, accessor util.NamespaceAccessor) (bool, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
	var output *unstructured.Unstructured
	var outputs []*unstructured.Unstructured
	var (
		appName  = appRev.Spec.Application.Name
		isHealth = true
	)
	if wl.CapabilityCategory == types.TerraformCategory {
		var configuration terraforv1beta2.Configuration
		if err := h.r.Client.Get(ctx, client.ObjectKey{Name: wl.Name, Namespace: accessor.Namespace()}, &configuration); err != nil {
			if kerrors.IsNotFound(err) {
				var legacyConfiguration terraforv1beta1.Configuration
				if err := h.r.Client.Get(ctx, client.ObjectKey{Name: wl.Name, Namespace: accessor.Namespace()}, &legacyConfiguration); err != nil {
					return false, nil, nil, errors.WithMessagef(err, "app=%s, comp=%s, check health error", appName, wl.Name)
				}
				isHealth = setStatus(status, legacyConfiguration.Status.ObservedGeneration, legacyConfiguration.Generation,
					legacyConfiguration.GetLabels(), appRev.Name, legacyConfiguration.Status.Apply.State, legacyConfiguration.Status.Apply.Message)
			} else {
				return false, nil, nil, errors.WithMessagef(err, "app=%s, comp=%s, check health error", appName, wl.Name)
			}
		} else {
			isHealth = setStatus(status, configuration.Status.ObservedGeneration, configuration.Generation, configuration.GetLabels(),
				appRev.Name, configuration.Status.Apply.State, configuration.Status.Apply.Message)
		}
	} else {
		templateContext, err := wl.GetTemplateContext(wl.Ctx, h.r.Client, accessor)
		if err != nil {
			return false, nil, nil, errors.WithMessagef(err, "app=%s, comp=%s, get template context error", appName, wl.Name)
		}
		if ok, err := wl.EvalHealth(templateContext); !ok || err != nil {
			isHealth = false
		}
		status.Healthy = isHealth
		status.Message, err = wl.EvalStatus(templateContext)
		if err != nil {
			return false, nil, nil, errors.WithMessagef(err, "app=%s, comp=%s, evaluate workload status message error", appName, wl.Name)
		}
		output, outputs = extractOutputAndOutputs(templateContext)
	}
	return isHealth, output, outputs, nil
}

// nolint
// collectHealthStatus will collect health status of component, including component itself and traits.
func (h *AppHandler) collectHealthStatus(ctx context.Context, wl *appfile.Workload, appRev *v1beta1.ApplicationRevision, overrideNamespace string, skipWorkload bool, traitFilters ...TraitFilter) (*common.ApplicationComponentStatus, *unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
	output := new(unstructured.Unstructured)
	outputs := make([]*unstructured.Unstructured, 0)
	accessor := util.NewApplicationResourceNamespaceAccessor(h.app.Namespace, overrideNamespace)
	var (
		status = common.ApplicationComponentStatus{
			Name:               wl.Name,
			WorkloadDefinition: wl.FullTemplate.Reference.Definition,
			Healthy:            true,
			Namespace:          accessor.Namespace(),
			Cluster:            multicluster.ClusterNameInContext(ctx),
		}
		isHealth = true
		err      error
	)

	status = h.getServiceStatus(status)
	if !skipWorkload {
		isHealth, output, outputs, err = h.collectWorkloadHealthStatus(ctx, wl, appRev, &status, accessor)
		if err != nil {
			return nil, nil, nil, false, err
		}
	}

	var traitStatusList []common.ApplicationTraitStatus
collectNext:
	for _, tr := range wl.Traits {
		for _, filter := range traitFilters {
			// If filtered out by one of the filters
			if filter(*tr) {
				continue collectNext
			}
		}

		traitStatus, _outputs, err := h.collectTraitHealthStatus(wl, tr, appRev, overrideNamespace)
		if err != nil {
			return nil, nil, nil, false, err
		}
		outputs = append(outputs, _outputs...)

		isHealth = isHealth && traitStatus.Healthy
		if status.Message == "" && traitStatus.Message != "" {
			status.Message = traitStatus.Message
		}
		traitStatusList = append(traitStatusList, traitStatus)

		var oldStatus []common.ApplicationTraitStatus
		for _, _trait := range status.Traits {
			if _trait.Type != tr.Name {
				oldStatus = append(oldStatus, _trait)
			}
		}
		status.Traits = oldStatus
	}
	status.Traits = append(status.Traits, traitStatusList...)
	status.Scopes = generateScopeReference(wl.Scopes)
	h.addServiceStatus(true, status)
	return &status, output, outputs, isHealth, nil
}

func setStatus(status *common.ApplicationComponentStatus, observedGeneration, generation int64, labels map[string]string,
	appRevName string, state terraformtypes.ConfigurationState, message string) bool {
	isLatest := func() bool {
		if observedGeneration != 0 && observedGeneration != generation {
			return false
		}
		// Use AppRevision to avoid getting the configuration before the patch.
		if v, ok := labels[oam.LabelAppRevision]; ok {
			if v != appRevName {
				return false
			}
		}
		return true
	}
	status.Message = message
	if !isLatest() || state != terraformtypes.Available {
		status.Healthy = false
		return false
	}
	status.Healthy = true
	return true
}

func generateScopeReference(scopes []appfile.Scope) []corev1.ObjectReference {
	var references []corev1.ObjectReference
	for _, scope := range scopes {
		references = append(references, corev1.ObjectReference{
			APIVersion: metav1.GroupVersion{
				Group:   scope.GVK.Group,
				Version: scope.GVK.Version,
			}.String(),
			Kind: scope.GVK.Kind,
			Name: scope.Name,
		})
	}
	return references
}

type garbageCollectFunc func(ctx context.Context, h *AppHandler) error

// execute garbage collection functions, including:
// - clean up legacy app revisions
// - clean up legacy component revisions
func garbageCollection(ctx context.Context, h *AppHandler) error {
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-rev").Observe(time.Since(t).Seconds())
	}()
	collectFuncs := []garbageCollectFunc{
		garbageCollectFunc(cleanUpApplicationRevision),
		garbageCollectFunc(cleanUpWorkflowComponentRevision),
	}
	for _, collectFunc := range collectFuncs {
		if err := collectFunc(ctx, h); err != nil {
			return err
		}
	}
	return nil
}

// ApplyPolicies will render policies into manifests from appfile and dispatch them
func (h *AppHandler) ApplyPolicies(ctx context.Context, af *appfile.Appfile) error {
	if ctx, ok := ctx.(monitorContext.Context); ok {
		subCtx := ctx.Fork("apply-policies", monitorContext.DurationMetric(func(v float64) {
			metrics.AppReconcileStageDurationHistogram.WithLabelValues("apply-policies").Observe(v)
		}))
		defer subCtx.Commit("finish apply policies")
	}
	policyManifests, err := af.GeneratePolicyManifests(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to render policy manifests")
	}
	if len(policyManifests) > 0 {
		for _, policyManifest := range policyManifests {
			util.AddLabels(policyManifest, map[string]string{
				oam.LabelAppName:      h.app.GetName(),
				oam.LabelAppNamespace: h.app.GetNamespace(),
			})
		}
		if err = h.Dispatch(ctx, "", common.PolicyResourceCreator, policyManifests...); err != nil {
			return errors.Wrapf(err, "failed to dispatch policy manifests")
		}
	}
	return nil
}

func extractOutputAndOutputs(templateContext map[string]interface{}) (*unstructured.Unstructured, []*unstructured.Unstructured) {
	output := new(unstructured.Unstructured)
	if templateContext["output"] != nil {
		output = &unstructured.Unstructured{Object: templateContext["output"].(map[string]interface{})}
	}
	outputs := extractOutputs(templateContext)
	return output, outputs
}

func extractOutputs(templateContext map[string]interface{}) []*unstructured.Unstructured {
	outputs := make([]*unstructured.Unstructured, 0)
	if templateContext["outputs"] != nil {
		for _, v := range templateContext["outputs"].(map[string]interface{}) {
			outputs = append(outputs, &unstructured.Unstructured{Object: v.(map[string]interface{})})
		}
	}
	return outputs
}
