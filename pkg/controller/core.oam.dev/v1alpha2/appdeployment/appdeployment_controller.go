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

package appdeployment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/pkg/errors"
	istioapiv1beta1 "istio.io/api/networking/v1beta1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/slice"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oamcorealpha "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/clustermanager"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	appDeploymentFinalizer = "finalizers.appdeployment.oam.dev"
	reconcileTimeOut       = 60 * time.Second
	secretKeyConfig        = "config"
)

var (
	errUpdateFinalizer = "error updating finalizer"
)

// Reconciler reconciles an AppDeployment object
type Reconciler struct {
	Client client.Client
	dm     discoverymapper.DiscoveryMapper
	wr     WorkloadRenderer
	Scheme *runtime.Scheme
}

// NewReconciler returns a new instance of Reconciler
func NewReconciler(cli client.Client, sch *runtime.Scheme, dm discoverymapper.DiscoveryMapper) *Reconciler {
	return &Reconciler{
		dm:     dm,
		Client: cli,
		Scheme: sch,
		wr:     NewWorkloadRenderer(cli),
	}
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=appdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=appdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=clusters/status,verbs=get;update;patch

// Reconcile is the main logic of appDeployment controller
func (r *Reconciler) Reconcile(req ctrl.Request) (res reconcile.Result, retErr error) {
	appDeployment := &oamcore.AppDeployment{}
	ctx, cancel := context.WithTimeout(context.TODO(), reconcileTimeOut)
	defer cancel()

	startTime := time.Now()
	defer func() {
		if retErr == nil {
			if res.Requeue || res.RequeueAfter > 0 {
				klog.InfoS("Finished reconciling appDeployment", "controller request", req, "time spent", time.Since(startTime), "result", res)
			} else {
				klog.InfoS("Finished reconcile appDeployment", "controller  request", req, "time spent", time.Since(startTime))
			}
		} else {
			klog.Errorf("Failed to reconcile appDeployment %s: %v", req, retErr)
		}
	}()

	if err := r.Client.Get(ctx, req.NamespacedName, appDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			klog.InfoS("appDeployment does not exist", "appDeployment", klog.KRef(req.Namespace, req.Name))
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	klog.InfoS("Start to reconcile", "appDeployment", klog.KObj(appDeployment))

	if !appDeployment.DeletionTimestamp.IsZero() {
		err := r.handleFinalizer(ctx, appDeployment)
		return ctrl.Result{}, err
	}

	// The object is not being deleted, so if it does not have our finalizer,
	// then lets add the finalizer and update the object. This is equivalent
	// registering our finalizer.
	if !slice.ContainsString(appDeployment.ObjectMeta.Finalizers, appDeploymentFinalizer, nil) {
		appDeployment.ObjectMeta.Finalizers = append(appDeployment.ObjectMeta.Finalizers, appDeploymentFinalizer)
		if err := r.Client.Update(context.Background(), appDeployment); err != nil {
			return ctrl.Result{}, err
		}
	}

	diff := r.calculateDiff(appDeployment)

	if !diff.Empty() {
		if appDeployment.Status.Phase != oamcore.PhaseRolling {
			appDeployment.Status.Phase = oamcore.PhaseRolling
			if err := r.updateStatus(ctx, appDeployment); err != nil {
				return ctrl.Result{}, err
			}
		}

		if err := r.deleteRevisions(ctx, appDeployment, diff.Del); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.applyRevisions(ctx, appDeployment, diff.Mod); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.applyRevisions(ctx, appDeployment, diff.Add); err != nil {
			return ctrl.Result{}, err
		}
	}

	appDeployment.Status.Phase = oamcore.PhaseCompleted
	appDeployment.Status.Placement = makePlacement(
		append(append(diff.Add, diff.Mod...), diff.Unchanged...),
	)

	if appDeployment.Spec.Traffic != nil {
		if err := r.applyTraffic(ctx, appDeployment); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, r.updateStatus(ctx, appDeployment)
}

func (r *Reconciler) handleFinalizer(ctx context.Context, appd *oamcore.AppDeployment) error {
	if !slice.ContainsString(appd.Finalizers, appDeploymentFinalizer, nil) {
		return nil
	}
	// our finalizer is present, so lets handle any external dependency
	if err := r.deleteExternalResources(ctx, appd); err != nil {
		// if fail to delete the external dependency here, return with error
		// so that it can be retried
		return err
	}

	appd.ObjectMeta.Finalizers = removeString(appd.ObjectMeta.Finalizers, appDeploymentFinalizer)
	return errors.Wrap(r.Client.Update(ctx, appd), errUpdateFinalizer)
}

func (r *Reconciler) deleteExternalResources(ctx context.Context, appd *oamcore.AppDeployment) error {
	var revsDel []*revision
	for _, p := range appd.Status.Placement {
		for _, c := range p.Clusters {
			if isHostCluster(c.ClusterName) {
				continue
			}
			revsDel = append(revsDel, newRevision(p.RevisionName, c.ClusterName, c.Replicas))
		}
	}

	return r.deleteRevisions(ctx, appd, revsDel)
}

func (r *Reconciler) getClientForCluster(ctx context.Context, cluster, ns string) (client.Client, error) {
	c, err := r.getCluster(ctx, cluster, ns)
	if err != nil {
		return nil, err
	}

	key := client.ObjectKey{
		Name:      c.Spec.KubeconfigSecretRef.Name,
		Namespace: ns,
	}
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	return clustermanager.GetClient(secret.Data[secretKeyConfig])
}

func (r *Reconciler) deleteRevisions(ctx context.Context, appd *oamcore.AppDeployment, revisions []*revision) (err error) {
	for _, rev := range revisions {
		klog.InfoS("delete revision", "revision", rev.RevisionName, "cluster", rev.ClusterName)

		workloads, err := r.getWorkloadsFromRevision(ctx, rev.RevisionName, appd.Namespace)
		if err != nil {
			return err
		}

		var kubecli client.Client
		if isHostCluster(rev.ClusterName) {
			kubecli = r.Client
		} else {
			kubecli, err = r.getClientForCluster(ctx, rev.ClusterName, appd.Namespace)
			if err != nil {
				return err
			}
		}

		for _, wl := range workloads {
			if err := kubecli.Delete(ctx, wl.Object); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			for _, tr := range wl.traits {
				if err := kubecli.Delete(ctx, tr.Object); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}

	}
	return nil
}

func isHostCluster(name string) bool {
	return name == ""
}

func (r *Reconciler) applyRevisions(ctx context.Context, appd *oamcore.AppDeployment, revisions []*revision) (err error) {
	for _, rev := range revisions {
		klog.InfoS("apply revision", "revision", rev.RevisionName, "cluster", rev.ClusterName)

		workloads, err := r.getWorkloadsFromRevision(ctx, rev.RevisionName, appd.Namespace)
		if err != nil {
			return err
		}

		var kubecli client.Client
		if isHostCluster(rev.ClusterName) {
			kubecli = r.Client
			addOwnerToWorkloads(appd, workloads)
		} else {
			kubecli, err = r.getClientForCluster(ctx, rev.ClusterName, appd.Namespace)
			if err != nil {
				return err
			}
		}

		if err := applyOverlayToWorkload(workloads,
			overlayReplica(rev.Replicas),
			overlayLabels(appd.Name, rev.RevisionName)); err != nil {
			return err
		}

		applicator := apply.NewAPIApplicator(kubecli)
		for _, wl := range workloads {
			if err := applicator.Apply(ctx, wl.Object); err != nil {
				return err
			}
			for _, tr := range wl.traits {
				if err := applicator.Apply(ctx, tr.Object); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func addOwnerToWorkloads(appd *oamcore.AppDeployment, workloads []*workload) {
	for _, wl := range workloads {
		addAppDeploymentAsOwner(wl.Object, appd)
		for _, tr := range wl.traits {
			addAppDeploymentAsOwner(tr.Object, appd)
		}
	}
}

func (r *Reconciler) getCluster(ctx context.Context, name, ns string) (*oamcore.Cluster, error) {
	var obj oamcore.Cluster
	key := client.ObjectKey{
		Name:      name,
		Namespace: ns,
	}
	if err := r.Client.Get(ctx, key, &obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (r *Reconciler) getWorkloadsFromRevision(ctx context.Context, revName, ns string) ([]*workload, error) {
	var appRev oamcore.ApplicationRevision
	key := client.ObjectKey{
		Name:      revName,
		Namespace: ns,
	}
	if err := r.Client.Get(ctx, key, &appRev); err != nil {
		return nil, err
	}

	ac, err := convertRawExtention2AppConfig(appRev.Spec.ApplicationConfiguration)
	if err != nil {
		return nil, err
	}

	// In AppRevision, none of the component have any revision in their names.
	appendRevisionToACComponentNames(ac, revName)

	var comps []*oamcorealpha.Component
	for _, rawComp := range appRev.Spec.Components {
		comp, err := convertRawExtention2Component(rawComp.Raw)
		if err != nil {
			return nil, err
		}
		comp.Name = makeRevisionName(comp.Name, revName)
		comps = append(comps, comp)
	}

	workloads, err := r.wr.Render(ctx, ac, comps)
	if err != nil {
		return nil, err
	}

	return workloads, nil
}

func appendRevisionToACComponentNames(ac *oamcorealpha.ApplicationConfiguration, revName string) {
	for i, acc := range ac.Spec.Components {
		compName := acc.ComponentName
		if acc.RevisionName != "" {
			compName = utils.ExtractComponentName(acc.RevisionName)
		}
		ac.Spec.Components[i].ComponentName = makeRevisionName(compName, revName)
	}
}

func convertRawExtention2AppConfig(raw runtime.RawExtension) (*oamcorealpha.ApplicationConfiguration, error) {
	obj := &oamcorealpha.ApplicationConfiguration{}
	b, err := raw.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func convertRawExtention2Component(raw runtime.RawExtension) (*oamcorealpha.Component, error) {
	obj := &oamcorealpha.Component{}
	b, err := raw.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func (r *Reconciler) calculateDiff(appd *oamcore.AppDeployment) *revisionsDiff {
	d := &revisionsDiff{}

	// Note: use (AC, cluster) as the key.
	curDict := make(map[revision]int)
	targetDict := make(map[revision]struct{})

	target := appd.Spec.AppRevisions

	for _, p := range appd.Status.Placement {

		for _, c := range p.Clusters {
			key := revision{
				RevisionName: p.RevisionName,
				ClusterName:  c.ClusterName,
			}
			curDict[key] = c.Replicas
		}
	}

	for _, rev := range target {
		for _, p := range rev.Placement {
			clusterName := ""
			if p.ClusterSelector != nil {
				clusterName = p.ClusterSelector.Name
			}
			key := revision{
				RevisionName: rev.RevisionName,
				ClusterName:  clusterName,
			}
			targetDict[key] = struct{}{}

			curReplicas, ok := curDict[key]

			toAdd := newRevision(rev.RevisionName, clusterName, p.Distribution.Replicas)
			if !ok {
				// need to add
				d.Add = append(d.Add, toAdd)
				continue
			}

			if p.Distribution.Replicas == curReplicas {
				d.Unchanged = append(d.Unchanged, toAdd)
				continue
			}
			// need to mod
			d.Mod = append(d.Mod, toAdd)
		}
	}

	for _, p := range appd.Status.Placement {
		for _, c := range p.Clusters {
			key := revision{
				RevisionName: p.RevisionName,
				ClusterName:  c.ClusterName,
			}

			_, ok := targetDict[key]
			if ok {
				continue
			}
			// need to del
			d.Del = append(d.Del, newRevision(p.RevisionName, c.ClusterName, c.Replicas))
		}
	}
	return d
}

// UpdateStatus updates AppDeployment's Status with retry.RetryOnConflict
func (r *Reconciler) updateStatus(ctx context.Context, appd *oamcore.AppDeployment, opts ...client.UpdateOption) error {
	status := appd.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Client.Get(ctx, client.ObjectKey{Namespace: appd.Namespace, Name: appd.Name}, appd); err != nil {
			return
		}
		appd.Status = status
		return r.Client.Status().Update(ctx, appd, opts...)
	})
}

// SetupWithManager setup the controller with manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oamcore.AppDeployment{}).
		Complete(r)
}

func (r *Reconciler) applyTraffic(ctx context.Context, appd *oamcore.AppDeployment) (err error) {

	vsvc := &istioclientv1beta1.VirtualService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: istioclientv1beta1.SchemeGroupVersion.String(),
			Kind:       "VirtualService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      appd.Name,
			Namespace: appd.Namespace,
		},
		Spec: istioapiv1beta1.VirtualService{
			Hosts:    appd.Spec.Traffic.Hosts,
			Gateways: appd.Spec.Traffic.Gateways,
		},
	}

	var svcs []*corev1.Service
	affectRevisions := map[string]struct{}{}
	for _, httpRule := range appd.Spec.Traffic.HTTP {
		var routes []*istioapiv1beta1.HTTPRouteDestination
		for _, target := range httpRule.WeightedTargets {
			affectRevisions[target.RevisionName] = struct{}{}
			svc := makeService(target.ComponentName, appd.Namespace, target.RevisionName, target.Port)
			svcs = append(svcs, svc)
			dst := &istioapiv1beta1.HTTPRouteDestination{
				Destination: &istioapiv1beta1.Destination{
					Host: svc.Name,
				},
				Weight: int32(target.Weight),
			}
			routes = append(routes, dst)
		}
		r := &istioapiv1beta1.HTTPRoute{
			Route: routes,
		}
		vsvc.Spec.Http = append(vsvc.Spec.Http, r)
	}

	affectedClusters := map[string]struct{}{}
	for _, placement := range appd.Status.Placement {
		_, ok := affectRevisions[placement.RevisionName]
		if !ok {
			continue
		}
		for _, cluster := range placement.Clusters {
			affectedClusters[cluster.ClusterName] = struct{}{}
		}
	}

	for clusterName := range affectedClusters {
		var kubecli client.Client
		if isHostCluster(clusterName) {
			kubecli = r.Client
			addAppDeploymentAsOwner(vsvc, appd)
			for _, svc := range svcs {
				addAppDeploymentAsOwner(svc, appd)
			}
		} else {
			kubecli, err = r.getClientForCluster(ctx, clusterName, appd.Namespace)
			if err != nil {
				return err
			}
		}

		applicator := apply.NewAPIApplicator(kubecli)
		if err := applicator.Apply(ctx, vsvc); err != nil {
			return err
		}
		for _, svc := range svcs {
			if err := applicator.Apply(ctx, svc); err != nil {
				return err
			}
		}
	}

	return nil
}

func addAppDeploymentAsOwner(child, appd metav1.Object) {
	child.SetOwnerReferences(append(child.GetOwnerReferences(),
		*metav1.NewControllerRef(appd, oamcore.AppDeploymentKindVersionKind)))
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// Setup adds a controller that reconciles AppDeployment.
func Setup(mgr ctrl.Manager, args controller.Args, _ logging.Logger) error {
	r := NewReconciler(mgr.GetClient(), mgr.GetScheme(), args.DiscoveryMapper)
	return r.SetupWithManager(mgr)
}
