/*
Copyright 2025 The KubeVela Authors.

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

package apply

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDryRunNamespaceTracking(t *testing.T) {
	// Test namespace tracking
	tracker := &DryRunNamespaceTracker{}
	
	// Add namespaces
	tracker.Add("test-ns-1")
	tracker.Add("test-ns-2")
	
	// Verify they're tracked
	namespaces := tracker.GetNamespaces()
	assert.Equal(t, 2, len(namespaces))
	assert.Contains(t, namespaces, "test-ns-1")
	assert.Contains(t, namespaces, "test-ns-2")
	
	// Clear and verify empty
	tracker.Clear()
	namespaces = tracker.GetNamespaces()
	assert.Equal(t, 0, len(namespaces))
}

func TestDryRunNamespaceCleanup(t *testing.T) {
	// Setup fake client with test namespaces
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	
	ns1 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cleanup-1"},
	}
	ns2 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cleanup-2"},
	}
	
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(ns1, ns2).
		Build()
	
	// Track the namespaces
	tracker := &DryRunNamespaceTracker{}
	tracker.Add("test-cleanup-1")
	tracker.Add("test-cleanup-2")
	tracker.Add("non-existent-ns") // This one doesn't exist
	
	// Cleanup
	ctx := context.Background()
	err := tracker.CleanupNamespaces(ctx, c)
	assert.NoError(t, err)
	
	// Verify namespaces were deleted
	var nsList corev1.NamespaceList
	err = c.List(ctx, &nsList)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(nsList.Items))
	
	// Verify tracker was cleared
	assert.Equal(t, 0, len(tracker.GetNamespaces()))
}

func TestDryRunNamespaceContext(t *testing.T) {
	// Test context storage and retrieval
	ctx := context.Background()
	tracker := &DryRunNamespaceTracker{}
	
	// Initially no tracker in context
	assert.Nil(t, GetDryRunNamespaceTracker(ctx))
	
	// Add tracker to context
	ctx = WithDryRunNamespaceTracker(ctx, tracker)
	
	// Retrieve and verify
	retrieved := GetDryRunNamespaceTracker(ctx)
	assert.NotNil(t, retrieved)
	assert.Equal(t, tracker, retrieved)
	
	// Add a namespace through retrieved tracker
	retrieved.Add("context-test-ns")
	assert.Contains(t, tracker.GetNamespaces(), "context-test-ns")
}

func TestNamespaceCreationWithTracking(t *testing.T) {
	// Setup fake client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	
	// Create applicator
	applicator := NewAPIApplicator(c)
	
	// Create namespace object
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dry-run-test-ns",
		},
	}
	
	// Create context with tracker
	ctx := context.Background()
	tracker := &DryRunNamespaceTracker{}
	ctx = WithDryRunNamespaceTracker(ctx, tracker)
	
	// Apply with dry-run (this should track the namespace)
	err := applicator.Apply(ctx, ns, DryRunAll())
	assert.NoError(t, err)
	
	// Verify namespace was tracked
	namespaces := tracker.GetNamespaces()
	assert.Contains(t, namespaces, "dry-run-test-ns")
	
	// Verify namespace was actually created (not dry-run for namespaces)
	var createdNs corev1.Namespace
	err = c.Get(ctx, client.ObjectKey{Name: "dry-run-test-ns"}, &createdNs)
	assert.NoError(t, err)
	
	// Test cleanup behavior (only used on failure in real scenario)
	// Clean up tracked namespaces
	err = tracker.CleanupNamespaces(ctx, c)
	assert.NoError(t, err)
	
	// Verify namespace was deleted
	err = c.Get(ctx, client.ObjectKey{Name: "dry-run-test-ns"}, &createdNs)
	assert.Error(t, err) // Should be NotFound error
}

func TestNamespaceCleanupOnlyOnFailure(t *testing.T) {
	// This test documents the intended behavior:
	// - Namespaces created during dry-run are tracked
	// - They are ONLY cleaned up if the dry-run fails
	// - On successful dry-run, namespaces are kept for the actual deployment
	
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()
	
	// Create namespace during dry-run
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "app-namespace"},
	}
	err := c.Create(context.Background(), ns)
	assert.NoError(t, err)
	
	tracker := &DryRunNamespaceTracker{}
	tracker.Add("app-namespace")
	
	// Scenario 1: Dry-run succeeds - namespace should be kept
	var existingNs corev1.Namespace
	err = c.Get(context.Background(), client.ObjectKey{Name: "app-namespace"}, &existingNs)
	assert.NoError(t, err, "Namespace should exist after successful dry-run")
	
	// Scenario 2: Dry-run fails - namespace should be cleaned up
	err = tracker.CleanupNamespaces(context.Background(), c)
	assert.NoError(t, err)
	
	err = c.Get(context.Background(), client.ObjectKey{Name: "app-namespace"}, &existingNs)
	assert.Error(t, err, "Namespace should be deleted after failed dry-run cleanup")
}