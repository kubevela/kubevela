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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

// AC4 + AC9: with the AddonCRD gate off (the default), Setup is a no-op and
// registers nothing. Passing a nil manager proves the gate short-circuits
// before any manager access: if the early return were missing, the nil manager
// would panic.
func TestSetupGateOffSkipsRegistration(t *testing.T) {
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
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(active).Build()
	r := &Reconciler{Client: cli, Scheme: scheme, options: options{reconcileInterval: time.Minute}}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: nn("active-addon")})
	assert.NoError(t, err)
	assert.Equal(t, time.Minute, res.RequeueAfter, "active CR must requeue after the configured interval")

	// Skeleton does no status writes yet.
	got := &v1beta1.Addon{}
	assert.NoError(t, cli.Get(context.Background(), nn("active-addon"), got))
	assert.Equal(t, v1beta1.AddonStatus{}, got.Status, "skeleton reconcile must not mutate status")
}

// AC6: when no interval is configured, Reconcile falls back to the default 5-minute interval.
func TestReconcileDefaultInterval(t *testing.T) {
	scheme := newTestScheme(t)
	active := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "default-interval"}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(active).Build()
	r := &Reconciler{Client: cli, Scheme: scheme} // no interval set

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
