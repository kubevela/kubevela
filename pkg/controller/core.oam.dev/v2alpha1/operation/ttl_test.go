/*
 Copyright 2026. The KubeVela Authors.

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

package operation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
)

func ttlTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v2alpha1.Operation{}).
		WithObjects(objs...).
		Build()
}

func TestSweepTTL(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()
	past := metav1.NewTime(now.Add(-time.Hour))

	testCases := map[string]struct {
		op                  *v2alpha1.Operation
		defaultOperationTTL time.Duration
		expectDeleted       bool
		expectRequeue       bool
	}{
		"no ttl at all: never deleted, never requeued": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "no-ttl", Namespace: "default"},
				Status:     v2alpha1.OperationStatus{CompletionTime: &past},
			},
		},
		"not yet terminal: ignored regardless of ttl": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "not-terminal", Namespace: "default"},
				Status:     v2alpha1.OperationStatus{},
			},
			defaultOperationTTL: time.Second,
		},
		"cluster default, not yet expired: requeues at the remaining window": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "not-expired", Namespace: "default"},
				Status:     v2alpha1.OperationStatus{CompletionTime: &now},
			},
			defaultOperationTTL: time.Hour,
			expectRequeue:       true,
		},
		"cluster default, expired: deleted": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "expired-default", Namespace: "default"},
				Status:     v2alpha1.OperationStatus{CompletionTime: &past},
			},
			defaultOperationTTL: time.Minute,
			expectDeleted:       true,
		},
		"per-operation override wins over cluster default: expired": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "expired-override", Namespace: "default"},
				Spec:       v2alpha1.OperationSpec{TTLSecondsAfterFinished: ptr.To(int32(30))},
				Status:     v2alpha1.OperationStatus{CompletionTime: &past},
			},
			defaultOperationTTL: 24 * time.Hour, // would NOT be expired under the cluster default
			expectDeleted:       true,
		},
		"explicit zero override disables ttl even with a cluster default": {
			op: &v2alpha1.Operation{
				ObjectMeta: metav1.ObjectMeta{Name: "explicit-zero", Namespace: "default"},
				Spec:       v2alpha1.OperationSpec{TTLSecondsAfterFinished: ptr.To(int32(0))},
				Status:     v2alpha1.OperationStatus{CompletionTime: &past},
			},
			defaultOperationTTL: time.Minute,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cli := ttlTestClient(t, tc.op)
			reconciler := &Reconciler{Client: cli, DefaultOperationTTL: tc.defaultOperationTTL}

			result, err := reconciler.sweepTTL(ctx, tc.op)
			r.NoError(err)

			if tc.expectRequeue {
				r.Greater(result.RequeueAfter, time.Duration(0))
			} else {
				r.Zero(result.RequeueAfter)
			}

			err = cli.Get(ctx, client.ObjectKeyFromObject(tc.op), &v2alpha1.Operation{})
			if tc.expectDeleted {
				r.True(kerrors.IsNotFound(err), "expected operation to be deleted, got err=%v", err)
			} else {
				r.NoError(err, "expected operation to still exist")
			}
		})
	}
}

func TestSweepTTLDoesNotDeleteARacedRestart(t *testing.T) {
	ctx := context.Background()
	r := require.New(t)
	past := metav1.NewTime(metav1.Now().Add(-time.Hour))

	op := &v2alpha1.Operation{
		ObjectMeta: metav1.ObjectMeta{Name: "raced-restart", Namespace: "default"},
		Status:     v2alpha1.OperationStatus{CompletionTime: &past},
	}
	cli := ttlTestClient(t, op)

	// Simulate a restart landing between the reconciler's stale read (via
	// APIReader, bypassing the cache) and this sweep call: mutate and
	// persist through the client, bumping the stored resourceVersion,
	// while op -- exactly like the stale value sweepTTL's caller would
	// still be holding -- keeps whatever resourceVersion it had before.
	live := &v2alpha1.Operation{}
	r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), live))
	live.Status.Phase = v2alpha1.OperationPhaseRunning
	live.Status.CompletionTime = nil
	r.NoError(cli.Status().Update(ctx, live))

	reconciler := &Reconciler{Client: cli, DefaultOperationTTL: time.Minute}
	_, err := reconciler.sweepTTL(ctx, op)
	r.NoError(err, "a resource-version conflict from stale data must not surface as a reconcile error")

	got := &v2alpha1.Operation{}
	r.NoError(cli.Get(ctx, client.ObjectKeyFromObject(op), got), "the restarted operation must survive a sweep based on stale (pre-restart) data")
	r.Equal(v2alpha1.OperationPhaseRunning, got.Status.Phase)
}
