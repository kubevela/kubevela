/*
Copyright 2024 The KubeVela Authors.

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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/features"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(scheme))
	return scheme
}

// AC4 + AC9: with the AddonCRD gate off, Setup is a no-op and registers nothing.
// Passing a nil manager proves the gate short-circuits before any manager
// access: if the early return were missing, the nil manager would panic. The
// gate is set explicitly here so the test does not depend on the default.
func TestSetupGateOffSkipsRegistration(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AddonCRD, false)
	assert.NotPanics(t, func() {
		err := Setup(nil, oamctrl.Args{})
		assert.NoError(t, err, "Setup must be a clean no-op when the AddonCRD gate is off")
	})
}

// AC4 + AC10 (gate-on half): with the gate on, Setup no longer short-circuits,
// so it proceeds past the gate and dereferences the (nil) manager — which
// panics. That panic is the observable proof the gate now permits registration.
func TestSetupGateOnProceedsPastGate(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AddonCRD, true)
	assert.Panics(t, func() {
		_ = Setup(nil, oamctrl.Args{})
	}, "with the gate on, Setup must proceed past the gate (and reach manager access)")
}

// AC8 + AC10: a reconcile for a CR that does not exist returns cleanly (no error, no requeue).
func TestReconcileNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: cli, Scheme: scheme}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("missing")})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res, "missing CR must not requeue")
}

// AC5 + AC10: a paused CR is skipped (requeued after the interval) with no status mutation.
func TestReconcilePausedSkipsWork(t *testing.T) {
	scheme := newTestScheme(t)
	paused := &v1beta1.Addon{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "paused-addon",
			Labels: map[string]string{"controller.core.oam.dev/pause": "true"},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(paused).Build()
	r := &Reconciler{Client: cli, Scheme: scheme, options: options{reconcileInterval: time.Minute}}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("paused-addon")})
	assert.NoError(t, err)
	assert.Equal(t, time.Minute, res.RequeueAfter, "paused CR must requeue after the configured interval")

	// No status mutation: status remains the zero value.
	got := &v1beta1.Addon{}
	assert.NoError(t, cli.Get(context.Background(), nn("paused-addon"), got))
	assert.Equal(t, v1beta1.AddonStatus{}, got.Status, "paused reconcile must not mutate status")
}

// AC6 + AC10: an active CR reconciles cleanly and reschedules itself after the interval.
func TestReconcileActiveRequeues(t *testing.T) {
	scheme := newTestScheme(t)
	active := &v1beta1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "active-addon", Generation: 3},
		Spec:       v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela"},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(active).
		WithStatusSubresource(&v1beta1.Addon{}).Build()
	r := &Reconciler{Client: cli, Scheme: scheme, options: options{reconcileInterval: time.Minute}}
	r.installFn = func(_ context.Context, _ *v1beta1.Addon) error { return nil }
	r.readBackFn = func(_ context.Context, a *v1beta1.Addon) error { a.Status.InstalledVersion = "v1.0.0"; return nil }

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("active-addon")})
	assert.NoError(t, err)
	assert.Equal(t, time.Minute, res.RequeueAfter, "active CR must requeue after the configured interval")
}

// AC6: when no interval is configured, Reconcile falls back to the default 5-minute interval.
func TestReconcileDefaultInterval(t *testing.T) {
	scheme := newTestScheme(t)
	active := &v1beta1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "default-interval"},
		Spec:       v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela"},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(active).
		WithStatusSubresource(&v1beta1.Addon{}).Build()
	r := &Reconciler{Client: cli, Scheme: scheme} // no interval set
	r.installFn = func(_ context.Context, _ *v1beta1.Addon) error { return nil }
	r.readBackFn = func(_ context.Context, a *v1beta1.Addon) error { a.Status.InstalledVersion = "v1.0.0"; return nil }

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("default-interval")})
	assert.NoError(t, err)
	assert.Equal(t, defaultReconcileInterval, res.RequeueAfter, "must fall back to the default interval")
}

// AC7: the field manager constant is defined and exported with the agreed value.
func TestFieldManagerConstant(t *testing.T) {
	assert.Equal(t, "addon.oam.dev/controller", FieldManagerAddonController)
}

func nn(name string) types.NamespacedName {
	return types.NamespacedName{Name: name}
}

func TestAddonRateLimiterCappedAt10m(t *testing.T) {
	rl := addonRateLimiter()
	req := ctrl.Request{NamespacedName: nn("x")}
	var last time.Duration
	for i := 0; i < 100; i++ {
		last = rl.When(req)
	}
	assert.LessOrEqual(t, last, 10*time.Minute, "backoff must cap at 10m")
	assert.GreaterOrEqual(t, last, 30*time.Second, "backoff must grow past the 30s base")
}

func newPhaseReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithStatusSubresource(&v1beta1.Addon{}).Build()
	return &Reconciler{Client: cli, Scheme: scheme, options: options{reconcileInterval: time.Minute}}
}

func TestReconcileFirstInstallReachesRunning(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd"}, Spec: v1beta1.AddonSpec{Version: "v1.2.0", Registry: "KubeVela"}}
	r := newPhaseReconciler(t, ad)
	r.installFn = func(ctx context.Context, a *v1beta1.Addon) error { return nil }
	r.readBackFn = func(ctx context.Context, a *v1beta1.Addon) error {
		a.Status.InstalledVersion = "v1.2.0"
		a.Status.ApplicationName = "addon-fluxcd"
		return nil
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	got := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), got))
	assert.Equal(t, v1beta1.AddonPhaseRunning, got.Status.Phase)
	assert.Equal(t, "v1.2.0", got.Status.InstalledVersion)
	assert.Equal(t, metav1.ConditionTrue, findCond(got, v1beta1.AddonConditionReady).Status)
	assert.Equal(t, metav1.ConditionTrue, findCond(got, v1beta1.AddonConditionSourceResolved).Status)
}

// A tracker present and owned, with no version change, must drive the heal path:
// the deleted auxiliary is re-created from the stored manifest and the network
// install (installFn) is never called.
func TestReconcileHealsWithoutInstall(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd", UID: "u1", Finalizers: []string{FinalizerAddonCleanup}}}
	r := newPhaseReconciler(t, ad)
	r.installFn = func(context.Context, *v1beta1.Addon) error {
		t.Fatal("install (network fetch) must not run on a steady-state heal")
		return nil
	}

	def := &unstructured.Unstructured{}
	def.SetGroupVersionKind(schema.FromAPIVersionAndKind("core.oam.dev/v1beta1", "ComponentDefinition"))
	def.SetNamespace("vela-system")
	def.SetName("helm-fluxcd")
	assert.NoError(t, r.writeTracker(context.Background(), ad, []client.Object{def}))

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)

	healed := &unstructured.Unstructured{}
	healed.SetGroupVersionKind(schema.FromAPIVersionAndKind("core.oam.dev/v1beta1", "ComponentDefinition"))
	assert.NoError(t, r.Get(context.Background(),
		client.ObjectKey{Namespace: "vela-system", Name: "helm-fluxcd"}, healed),
		"deleted auxiliary must be healed from the tracker")

	got := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), got))
	assert.Equal(t, v1beta1.AddonPhaseRunning, got.Status.Phase)
}

func TestReconcileUpgradePhase(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd"}, Spec: v1beta1.AddonSpec{Version: "v2.0.0", Registry: "KubeVela"}}
	ad.Status.InstalledVersion = "v1.2.0"
	ad.Status.Phase = v1beta1.AddonPhaseRunning
	r := newPhaseReconciler(t, ad)
	var sawPhase v1beta1.AddonPhase
	r.installFn = func(ctx context.Context, a *v1beta1.Addon) error { sawPhase = a.Status.Phase; return nil }
	r.readBackFn = func(ctx context.Context, a *v1beta1.Addon) error { a.Status.InstalledVersion = "v2.0.0"; return nil }
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	assert.Equal(t, v1beta1.AddonPhaseUpgrading, sawPhase, "phase must be upgrading before delegating")
}

func TestReconcileInstallErrorFails(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd"}, Spec: v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela"}}
	r := newPhaseReconciler(t, ad)
	r.installFn = func(ctx context.Context, a *v1beta1.Addon) error { return errors.New("render failed") }
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.Error(t, err)
	got := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), got))
	assert.Equal(t, v1beta1.AddonPhaseFailed, got.Status.Phase)
	// A non-registry install failure means the source resolved; SourceResolved
	// must reflect that, not a stale RegistryUnreachable from an earlier outage.
	assert.Equal(t, metav1.ConditionTrue, findCond(got, v1beta1.AddonConditionSourceResolved).Status)
	assert.Equal(t, metav1.ConditionFalse, findCond(got, v1beta1.AddonConditionReady).Status)
}

func TestReconcileRegistryUnreachableKeepsPhase(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd"}, Spec: v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela"}}
	ad.Status.Phase = v1beta1.AddonPhaseRunning
	ad.Status.InstalledVersion = "v1.0.0"
	r := newPhaseReconciler(t, ad)
	r.installFn = func(ctx context.Context, a *v1beta1.Addon) error {
		return fmt.Errorf("%w: dial timeout", errSourceUnresolved)
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.Error(t, err, "must return error to trigger backoff")
	got := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), got))
	assert.Equal(t, v1beta1.AddonPhaseRunning, got.Status.Phase, "transient outage must not change phase")
	assert.Equal(t, metav1.ConditionFalse, findCond(got, v1beta1.AddonConditionSourceResolved).Status)
	assert.Equal(t, "RegistryUnreachable", findCond(got, v1beta1.AddonConditionSourceResolved).Reason)
}

func TestReconcileRegistryUnreachableEscalatesAfterWindow(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd"}, Spec: v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela"}}
	ad.Status.Phase = v1beta1.AddonPhaseRunning
	ad.Status.Conditions = []metav1.Condition{{
		Type:               v1beta1.AddonConditionSourceResolved,
		Status:             metav1.ConditionFalse,
		Reason:             "RegistryUnreachable",
		LastTransitionTime: metav1.NewTime(time.Now().Add(-11 * time.Minute)),
	}}
	r := newPhaseReconciler(t, ad)
	r.installFn = func(ctx context.Context, a *v1beta1.Addon) error {
		return fmt.Errorf("%w: dial timeout", errSourceUnresolved)
	}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.Error(t, err)
	got := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), got))
	assert.Equal(t, v1beta1.AddonPhaseFailed, got.Status.Phase, "persistent failure past window escalates to failed")
}

func TestReconcileIdempotentNoChurn(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd", Generation: 1}, Spec: v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela"}}
	r := newPhaseReconciler(t, ad)
	r.installFn = func(ctx context.Context, a *v1beta1.Addon) error { return nil }
	r.readBackFn = func(ctx context.Context, a *v1beta1.Addon) error { a.Status.InstalledVersion = "v1.0.0"; return nil }
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	first := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), first))
	rv1 := first.ResourceVersion
	_, err = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fluxcd")})
	assert.NoError(t, err)
	second := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fluxcd"), second))
	assert.Equal(t, rv1, second.ResourceVersion, "no spec change -> no status write -> resourceVersion stable")
}

func TestReconcileAppliesCleanupFinalizer(t *testing.T) {
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fin-addon"}, Spec: v1beta1.AddonSpec{Version: "v1.0.0", Registry: "KubeVela"}}
	r := newPhaseReconciler(t, ad)
	r.installFn = func(ctx context.Context, a *v1beta1.Addon) error { return nil }
	r.readBackFn = func(ctx context.Context, a *v1beta1.Addon) error { return nil }

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fin-addon")})
	assert.NoError(t, err)

	got := &v1beta1.Addon{}
	assert.NoError(t, r.Get(context.Background(), nn("fin-addon"), got))
	assert.True(t, controllerutil.ContainsFinalizer(got, FinalizerAddonCleanup), "live CR must carry the cleanup finalizer")

	_, err = r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("fin-addon")})
	assert.NoError(t, err)
	assert.NoError(t, r.Get(context.Background(), nn("fin-addon"), got))
	count := 0
	for _, f := range got.Finalizers {
		if f == FinalizerAddonCleanup {
			count++
		}
	}
	assert.Equal(t, 1, count, "finalizer must not be added twice")
}
