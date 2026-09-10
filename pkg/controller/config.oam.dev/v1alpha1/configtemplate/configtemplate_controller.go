/*
Copyright 2026 The KubeVela Authors.

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

package configtemplate

import (
	"context"
	"encoding/json"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// Reconciler reconciles a ConfigTemplate object.
type Reconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	record               event.Recorder
	concurrentReconciles int
}

// Reconcile is the main logic for the ConfigTemplate controller.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.InfoS("Reconciling ConfigTemplate", "name", req.Name, "namespace", req.Namespace)

	var ct configv1alpha1.ConfigTemplate
	if err := r.Get(ctx, req.NamespacedName, &ct); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// no finalizer: ConfigTemplate has no external resources to clean up
	if ct.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	cueScript := script.CUE(ct.Spec.Template)
	if _, err := cueScript.ParseToTemplateValueWithCueX(ctx); err != nil {
		return r.markError(ctx, &ct, err)
	}

	schema, err := cueScript.ParsePropertiesToSchemaWithCueX(ctx, "template")
	if err != nil {
		return r.markError(ctx, &ct, err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return r.markError(ctx, &ct, err)
	}

	ct.Status.Schema = &runtime.RawExtension{Raw: raw}
	ct.Status.Phase = configv1alpha1.ConfigTemplatePhaseAvailable
	ct.Status.SetConditions(condition.ReconcileSuccess())
	return ctrl.Result{}, r.UpdateStatus(ctx, &ct)
}

// markError records a parse/schema-extraction failure on the status.
func (r *Reconciler) markError(ctx context.Context, ct *configv1alpha1.ConfigTemplate, err error) (ctrl.Result, error) {
	ct.Status.Phase = configv1alpha1.ConfigTemplatePhaseError
	ct.Status.SetConditions(condition.ReconcileError(err))
	if uerr := r.UpdateStatus(ctx, ct); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{}, nil
}

// UpdateStatus updates ConfigTemplate's Status with retry.RetryOnConflict
func (r *Reconciler) UpdateStatus(ctx context.Context, ct *configv1alpha1.ConfigTemplate, opts ...client.SubResourceUpdateOption) error {
	status := ct.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = r.Get(ctx, client.ObjectKey{Namespace: ct.Namespace, Name: ct.Name}, ct); err != nil {
			return
		}
		ct.Status = status
		return r.Status().Update(ctx, ct, opts...)
	})
}

// SetupWithManager will setup with event recorder
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("ConfigTemplate")).
		WithAnnotations("controller", "ConfigTemplate")
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.concurrentReconciles,
		}).
		For(&configv1alpha1.ConfigTemplate{}).
		Complete(r)
}

// Setup adds a controller that reconciles ConfigTemplate.
func Setup(mgr ctrl.Manager, args oamctrl.Args) error {
	r := Reconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		concurrentReconciles: args.ConcurrentReconciles,
	}
	return r.SetupWithManager(mgr)
}
