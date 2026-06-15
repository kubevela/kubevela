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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// newDeletingReconciler builds a reconciler whose Addon CR is mid-deletion: it
// carries the cleanup finalizer and a deletionTimestamp (set by issuing Delete,
// which the fake client honors because the finalizer keeps the object around).
func newDeletingReconciler(t *testing.T, policy v1beta1.AddonDeletionPolicy) *Reconciler {
	t.Helper()
	scheme := newTestScheme(t)
	ad := &v1beta1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "fluxcd", Finalizers: []string{FinalizerAddonCleanup}},
		Spec:       v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela", DeletionPolicy: policy},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ad).
		WithStatusSubresource(&v1beta1.Addon{}).Build()
	r := &Reconciler{Client: cli, Scheme: scheme, options: options{reconcileInterval: time.Minute}}
	assert.NoError(t, cli.Delete(context.Background(), ad))
	return r
}

func TestHandleDeletionOrphanLeavesAppAndReleases(t *testing.T) {
	r := newDeletingReconciler(t, v1beta1.AddonDeletionPolicyOrphan)
	called := false
	r.disableFn = func(ctx context.Context, a *v1beta1.Addon, force bool) error { called = true; return nil }

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	assert.False(t, called, "Orphan must not delete the owned Application")

	got := &v1beta1.Addon{}
	assert.True(t, apierrors.IsNotFound(r.Get(context.Background(), nn("fluxcd"), got)),
		"finalizer released -> CR removed")
}

func TestHandleDeletionForceTearsDownAndReleases(t *testing.T) {
	r := newDeletingReconciler(t, v1beta1.AddonDeletionPolicyForce)
	var sawForce bool
	var calls int
	r.disableFn = func(ctx context.Context, a *v1beta1.Addon, force bool) error { calls++; sawForce = force; return nil }

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.True(t, sawForce, "Force must call disable with force=true")

	got := &v1beta1.Addon{}
	assert.True(t, apierrors.IsNotFound(r.Get(context.Background(), nn("fluxcd"), got)),
		"CR removed after teardown")
}

func TestHandleDeletionForceTreatsNotFoundAsDone(t *testing.T) {
	r := newDeletingReconciler(t, v1beta1.AddonDeletionPolicyForce)
	r.disableFn = func(ctx context.Context, a *v1beta1.Addon, force bool) error {
		return apierrors.NewNotFound(v1beta1.SchemeGroupVersion.WithResource("applications").GroupResource(), "addon-fluxcd")
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	got := &v1beta1.Addon{}
	assert.True(t, apierrors.IsNotFound(r.Get(context.Background(), nn("fluxcd"), got)),
		"already-gone Application -> finalizer still released")
}

func TestHandleDeletionProtectCleanReleases(t *testing.T) {
	r := newDeletingReconciler(t, v1beta1.AddonDeletionPolicyProtect)
	r.disableFn = func(ctx context.Context, a *v1beta1.Addon, force bool) error {
		assert.False(t, force, "Protect must call disable with force=false")
		return nil
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	got := &v1beta1.Addon{}
	assert.True(t, apierrors.IsNotFound(r.Get(context.Background(), nn("fluxcd"), got)),
		"no dependents -> CR removed")
}

func TestHandleDeletionProtectBlockedKeepsFinalizer(t *testing.T) {
	r := newDeletingReconciler(t, v1beta1.AddonDeletionPolicyProtect)
	r.disableFn = func(ctx context.Context, a *v1beta1.Addon, force bool) error {
		return errors.New("addon fluxcd is in use by applications: default/myapp")
	}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err, "a blocked delete requeues rather than erroring")
	assert.Equal(t, time.Minute, res.RequeueAfter)

	got := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), got), "CR must remain (finalizer retained)")
	assert.True(t, controllerutil.ContainsFinalizer(got, FinalizerAddonCleanup))
	assert.Equal(t, "DeletionBlocked", findCond(got, v1beta1.AddonConditionReady).Reason)
	assert.Equal(t, v1beta1.AddonPhaseDeleting, got.Status.Phase, "a blocked delete surfaces phase=deleting")
}

func TestHandleDeletionEmptyPolicyDefaultsToProtect(t *testing.T) {
	r := newDeletingReconciler(t, "")
	var sawForce bool
	r.disableFn = func(ctx context.Context, a *v1beta1.Addon, force bool) error { sawForce = force; return nil }
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	assert.False(t, sawForce, "empty deletionPolicy must behave as Protect (force=false)")
}
