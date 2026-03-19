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
	"testing"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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
