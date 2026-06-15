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

package addon

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
)

// disable deletes the addon's owned Application through the shared installer
// path (the same DisableAddon `vela addon disable` uses). force skips the
// in-use dependency check; without it the call fails while other Applications
// still reference the addon's definitions.
func (r *Reconciler) disable(ctx context.Context, ad *v1beta1.Addon, force bool) error {
	return pkgaddon.DisableAddon(ctx, r.Client, ad.Name, r.Config, force)
}

// handleDeletion runs while the Addon CR is being deleted. It applies
// spec.deletionPolicy and then releases the cleanup finalizer so Kubernetes can
// remove the CR:
//   - Orphan:  leave the owned Application and its resources running.
//   - Force:   delete the owned Application regardless of dependents.
//   - Protect: delete only when no Application references the addon's
//     definitions; otherwise keep the finalizer and retry.
func (r *Reconciler) handleDeletion(ctx context.Context, ad *v1beta1.Addon) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ad, FinalizerAddonCleanup) {
		// Nothing of ours left to clean up; let the delete proceed.
		return ctrl.Result{}, nil
	}

	policy := ad.Spec.DeletionPolicy
	if policy == "" {
		policy = v1beta1.AddonDeletionPolicyProtect
	}

	switch policy {
	case v1beta1.AddonDeletionPolicyOrphan:
		return r.releaseFinalizer(ctx, ad)

	case v1beta1.AddonDeletionPolicyForce:
		// NotFound means the Application is already gone — treat as success.
		if err := r.disableFn(ctx, ad, true); err != nil && !apierrors.IsNotFound(err) {
			return r.deletionFailed(ctx, ad, "DeletionFailed", err)
		}
		return r.releaseFinalizer(ctx, ad)

	default: // Protect
		err := r.disableFn(ctx, ad, false)
		if err == nil || apierrors.IsNotFound(err) {
			return r.releaseFinalizer(ctx, ad)
		}
		// Dependents still reference the addon (or another error): keep the
		// finalizer, surface why, and retry on the next interval rather than
		// erroring (a blocked delete is an expected steady state, not a fault).
		base := ad.DeepCopy()
		setPhase(ad, v1beta1.AddonPhaseDeleting)
		setCondition(ad, v1beta1.AddonConditionReady, metav1.ConditionFalse, "DeletionBlocked", err.Error())
		if perr := r.patchStatus(ctx, base, ad); perr != nil {
			return ctrl.Result{}, perr
		}
		return ctrl.Result{RequeueAfter: r.interval()}, nil
	}
}

// releaseFinalizer removes the cleanup finalizer so the CR can be deleted.
func (r *Reconciler) releaseFinalizer(ctx context.Context, ad *v1beta1.Addon) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(ad, FinalizerAddonCleanup)
	return ctrl.Result{}, r.Update(ctx, ad)
}

// deletionFailed records a teardown failure on the CR and retries via the rate
// limiter (the finalizer stays, so the CR remains until cleanup succeeds).
func (r *Reconciler) deletionFailed(ctx context.Context, ad *v1beta1.Addon, reason string, err error) (ctrl.Result, error) {
	base := ad.DeepCopy()
	setPhase(ad, v1beta1.AddonPhaseDeleting)
	setCondition(ad, v1beta1.AddonConditionReady, metav1.ConditionFalse, reason, err.Error())
	if perr := r.patchStatus(ctx, base, ad); perr != nil {
		return ctrl.Result{}, perr
	}
	return ctrl.Result{}, err
}
