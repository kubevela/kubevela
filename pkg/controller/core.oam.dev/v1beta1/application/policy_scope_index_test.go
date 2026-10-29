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

package application

import (
	"context"
	"reflect"
	"testing"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlEvent "sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// errorClient wraps a fake client and forces List calls to fail.
type errorClient struct {
	client.Client
}

func (e *errorClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return errors.New("simulated API server error")
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1beta1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add v1beta1 to scheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	return s
}

func TestPolicyScopeIndexRetryOnBootstrapFailure(t *testing.T) {
	idx := NewPolicyScopeIndex()

	// Inject a client that will fail List calls
	idx.client = &errorClient{Client: fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()}

	// First call: initializeLocked fails, so initialized must remain false
	idx.ensureInitialized(context.Background())
	if idx.initialized {
		t.Fatal("expected initialized=false after bootstrap failure, got true")
	}

	// Second call: replace with a working client (no PolicyDefinitions in cluster)
	idx.client = fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()
	idx.ensureInitialized(context.Background())
	if !idx.initialized {
		t.Fatal("expected initialized=true after successful bootstrap, got false")
	}
}

func TestPolicyScopeIndexNoRetryAfterSuccess(t *testing.T) {
	idx := NewPolicyScopeIndex()
	idx.client = fake.NewClientBuilder().WithScheme(newTestScheme(t)).Build()

	// First call succeeds
	idx.ensureInitialized(context.Background())
	if !idx.initialized {
		t.Fatal("expected initialized=true after success")
	}

	// Replace with failing client — subsequent calls must NOT re-initialize
	idx.client = &errorClient{}
	idx.ensureInitialized(context.Background()) // must be a no-op
	if !idx.initialized {
		t.Fatal("expected initialized=true to be sticky after success")
	}
}

func TestPolicyScopeIndexDeleteRemovesEntry(t *testing.T) {
	idx := NewPolicyScopeIndex()

	policy := &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-policy",
			Namespace: "my-ns",
		},
		Spec: v1beta1.PolicyDefinitionSpec{
			Scope: v1beta1.ApplicationScope,
		},
	}

	idx.AddOrUpdate(policy)
	if idx.GetFromNamespace("my-policy", "my-ns") == nil {
		t.Fatal("expected entry after AddOrUpdate, got nil")
	}

	idx.Delete("my-policy", "my-ns")
	if idx.GetFromNamespace("my-policy", "my-ns") != nil {
		t.Fatal("expected nil after Delete, got entry")
	}
}

func TestPolicyScopeIndexDeleteOnDeletionTimestamp(t *testing.T) {
	// Simulates handlePolicyDefinitionChange receiving a delete event:
	// DeletionTimestamp is set, so Delete should be called instead of AddOrUpdate.
	idx := NewPolicyScopeIndex()

	now := metav1.Now()
	policy := &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleted-policy",
			Namespace:         "my-ns",
			DeletionTimestamp: &now,
			Finalizers:        []string{"test"},
		},
		Spec: v1beta1.PolicyDefinitionSpec{
			Scope: v1beta1.ApplicationScope,
		},
	}

	// Pre-populate the index as if the policy existed before the delete event
	idx.AddOrUpdate(&v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "deleted-policy", Namespace: "my-ns"},
		Spec:       v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope},
	})

	// Simulate what handlePolicyDefinitionChange now does
	if policy.GetDeletionTimestamp() != nil {
		idx.Delete(policy.Name, policy.Namespace)
	} else {
		idx.AddOrUpdate(policy)
	}

	if idx.GetFromNamespace("deleted-policy", "my-ns") != nil {
		t.Fatal("expected entry to be removed when DeletionTimestamp is set")
	}
}

// useTestPolicyScopeIndex swaps the package-global index for an empty one for the duration of t.
func useTestPolicyScopeIndex(t *testing.T) {
	t.Helper()
	original := policyScopeIndex
	t.Cleanup(func() { policyScopeIndex = original })
	policyScopeIndex = NewPolicyScopeIndex()
	policyScopeIndex.initialized = true
}

func newGlobalTestPolicy(namespace, source string) *v1beta1.PolicyDefinition {
	return &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-global", Namespace: namespace},
		Spec: v1beta1.PolicyDefinitionSpec{
			Scope:    v1beta1.ApplicationScope,
			Global:   true,
			Priority: 10,
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: `parameter: {}, output: labels: {"policy-source": "` + source + `"}`},
			},
		},
	}
}

func TestPolicyDefinitionWatchDeleteWithoutFinalizer(t *testing.T) {
	useTestPolicyScopeIndex(t)
	systemPolicy := newGlobalTestPolicy(oam.SystemDefinitionNamespace, "vela-system")
	nsPolicy := newGlobalTestPolicy("my-ns", "my-ns")
	policyScopeIndex.AddOrUpdate(systemPolicy)
	policyScopeIndex.AddOrUpdate(nsPolicy)

	if nsPolicy.GetDeletionTimestamp() != nil {
		t.Fatal("test precondition: delete event must carry a nil DeletionTimestamp")
	}
	r := &Reconciler{}
	r.policyDefinitionEventHandler().Delete(context.Background(), ctrlEvent.DeleteEvent{Object: nsPolicy}, nil)

	if policyScopeIndex.GetFromNamespace("shared-global", "my-ns") != nil {
		t.Fatal("expected namespace policy to be evicted from index on delete event with nil DeletionTimestamp")
	}
	deduped := policyScopeIndex.GetGlobalApplicationPoliciesDeduped("my-ns")
	if len(deduped) != 1 || deduped[0].Namespace != oam.SystemDefinitionNamespace {
		t.Fatalf("expected fallback to vela-system global policy after namespace policy deletion, got %+v", deduped)
	}
}

func TestRenderPoliciesForNewRevisionStaleNamespacePolicy(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableApplicationScopedPolicies, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableGlobalPolicies, true)

	tests := map[string]struct {
		withSystemPolicy bool
		wantNamespaces   []string
	}{
		"falls back to same-named vela-system policy": {
			withSystemPolicy: true,
			wantNamespaces:   []string{oam.SystemDefinitionNamespace},
		},
		"skips the policy when there is no vela-system fallback": {
			withSystemPolicy: false,
			wantNamespaces:   nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			useTestPolicyScopeIndex(t)
			builder := fake.NewClientBuilder().WithScheme(newTestScheme(t))
			if tt.withSystemPolicy {
				systemPolicy := newGlobalTestPolicy(oam.SystemDefinitionNamespace, "vela-system")
				builder = builder.WithObjects(systemPolicy)
				policyScopeIndex.AddOrUpdate(systemPolicy)
			}
			cli := builder.Build()
			// The namespace policy is deleted from the cluster, but the watch event has not
			// reached the index yet.
			policyScopeIndex.AddOrUpdate(newGlobalTestPolicy("my-ns", "my-ns"))

			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "my-ns"}}
			h := &AppHandler{Client: cli, app: app, isNewRevision: true}
			results, err := h.renderPoliciesForNewRevision(monitorContext.NewTraceContext(context.Background(), "test"), app)
			if err != nil {
				t.Fatalf("unexpected error rendering policies: %v", err)
			}
			var gotNamespaces []string
			for _, res := range results {
				gotNamespaces = append(gotNamespaces, res.PolicyNamespace)
			}
			if !reflect.DeepEqual(gotNamespaces, tt.wantNamespaces) {
				t.Fatalf("rendered policy namespaces: got %v, want %v", gotNamespaces, tt.wantNamespaces)
			}
			if policyScopeIndex.GetFromNamespace("shared-global", "my-ns") == nil {
				t.Fatal("render must not evict index entries; eviction belongs to the watch handler")
			}
		})
	}
}

func TestCreateOrUpdateDiffsConfigMap_UpdateRefreshesOwner(t *testing.T) {
	ctx := context.Background()
	s := newTestScheme(t)

	oldUID := types.UID("old-uid-111")
	newUID := types.UID("new-uid-222")
	appName := "my-app"
	appNS := "default"

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: appNS,
			UID:       newUID,
		},
	}

	cmName := policyConfigMapName(appNS, appName)

	// Pre-create ConfigMap with stale UID (simulates Application deleted and recreated)
	stale := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: appNS,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         v1beta1.SchemeGroupVersion.String(),
					Kind:               v1beta1.ApplicationKind,
					Name:               appName,
					UID:                oldUID,
					Controller:         ptrBool(true),
					BlockOwnerDeletion: ptrBool(true),
				},
			},
			Labels: map[string]string{
				oam.LabelAppUID: string(oldUID),
			},
		},
		Data: map[string]string{"old-key": "old-val"},
	}

	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(stale).Build()

	if err := createOrUpdateDiffsConfigMap(ctx, cli, app, map[string]string{"new-key": "new-val"}); err != nil {
		t.Fatalf("createOrUpdateDiffsConfigMap: %v", err)
	}

	updated := &corev1.ConfigMap{}
	if err := cli.Get(ctx, client.ObjectKey{Name: cmName, Namespace: appNS}, updated); err != nil {
		t.Fatalf("Get updated ConfigMap: %v", err)
	}

	if updated.Data["new-key"] != "new-val" {
		t.Errorf("Data not updated: %v", updated.Data)
	}
	if len(updated.OwnerReferences) != 1 || updated.OwnerReferences[0].UID != newUID {
		t.Errorf("OwnerReference UID: got %v, want %q", updated.OwnerReferences, newUID)
	}
	if got := updated.Labels[oam.LabelAppUID]; got != string(newUID) {
		t.Errorf("LabelAppUID: got %q, want %q", got, string(newUID))
	}
}

func TestRecordPolicyStatuses_HasContext(t *testing.T) {
	tests := []struct {
		name           string
		additionalCtx  map[string]interface{}
		wantHasContext bool
	}{
		{
			name:           "non-empty AdditionalContext sets HasContext=true",
			additionalCtx:  map[string]interface{}{"tenant": "acme"},
			wantHasContext: true,
		},
		{
			name:           "nil AdditionalContext leaves HasContext=false",
			additionalCtx:  nil,
			wantHasContext: false,
		},
		{
			name:           "empty AdditionalContext leaves HasContext=false",
			additionalCtx:  map[string]interface{}{},
			wantHasContext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &v1beta1.Application{}
			results := []RenderedPolicyResult{
				{
					PolicyName:        "my-policy",
					Enabled:           true,
					AdditionalContext: tt.additionalCtx,
					Transforms:        &PolicyOutput{},
				},
			}
			recordPolicyStatuses(app, results)
			if len(app.Status.AppliedApplicationPolicies) != 1 {
				t.Fatalf("expected 1 status entry, got %d", len(app.Status.AppliedApplicationPolicies))
			}
			got := app.Status.AppliedApplicationPolicies[0].HasContext
			if got != tt.wantHasContext {
				t.Errorf("HasContext: got %v, want %v", got, tt.wantHasContext)
			}
		})
	}
}

func TestPolicyDefinitionEventHandler(t *testing.T) {
	ctx := context.Background()

	useTestPolicyScopeIndex(t)

	r := &Reconciler{}
	h := r.policyDefinitionEventHandler()

	newPolicy := func(namespace string, priority int32) *v1beta1.PolicyDefinition {
		return &v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "evt-policy", Namespace: namespace},
			Spec: v1beta1.PolicyDefinitionSpec{
				Scope:    v1beta1.ApplicationScope,
				Global:   true,
				Priority: priority,
			},
		}
	}

	for _, ns := range []string{oam.SystemDefinitionNamespace, "my-ns"} {
		t.Run(ns, func(t *testing.T) {
			created := newPolicy(ns, 10)
			h.Create(ctx, ctrlEvent.CreateEvent{Object: created}, nil)
			if got := policyScopeIndex.GetFromNamespace("evt-policy", ns); got == nil || got.Priority != 10 {
				t.Fatalf("expected create event to index policy with priority 10, got %+v", got)
			}

			updated := newPolicy(ns, 20)
			h.Update(ctx, ctrlEvent.UpdateEvent{ObjectOld: created, ObjectNew: updated}, nil)
			if got := policyScopeIndex.GetFromNamespace("evt-policy", ns); got == nil || got.Priority != 20 {
				t.Fatalf("expected update event to refresh policy priority to 20, got %+v", got)
			}

			h.Delete(ctx, ctrlEvent.DeleteEvent{Object: updated}, nil)
			if got := policyScopeIndex.GetFromNamespace("evt-policy", ns); got != nil {
				t.Fatalf("expected delete event to evict policy without finalizers, got %+v", got)
			}
		})
	}

	t.Run("update with DeletionTimestamp evicts policy", func(t *testing.T) {
		h.Create(ctx, ctrlEvent.CreateEvent{Object: newPolicy("my-ns", 10)}, nil)
		terminating := newPolicy("my-ns", 10)
		now := metav1.Now()
		terminating.DeletionTimestamp = &now
		terminating.Finalizers = []string{"test"}
		h.Update(ctx, ctrlEvent.UpdateEvent{ObjectOld: newPolicy("my-ns", 10), ObjectNew: terminating}, nil)
		if got := policyScopeIndex.GetFromNamespace("evt-policy", "my-ns"); got != nil {
			t.Fatalf("expected terminating policy to be evicted, got %+v", got)
		}
	})

	t.Run("non-PolicyDefinition object is ignored", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "evt-policy", Namespace: "my-ns"}}
		h.Create(ctx, ctrlEvent.CreateEvent{Object: cm}, nil)
		h.Delete(ctx, ctrlEvent.DeleteEvent{Object: cm}, nil)
		if got := policyScopeIndex.GetFromNamespace("evt-policy", "my-ns"); got != nil {
			t.Fatalf("expected non-PolicyDefinition object to be ignored, got %+v", got)
		}
	})
}
