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

package framework

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateFreshNamespaceRejectsCollisionWithoutDeleting(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "occupied", Labels: map[string]string{"owner": "someone-else"}}},
	).Build()
	if _, err := CreateFreshNamespace(ctx, cli, "occupied"); err == nil {
		t.Fatal("existing namespace must be reported as a collision")
	}
	var existing corev1.Namespace
	if err := cli.Get(ctx, client.ObjectKey{Name: "occupied"}, &existing); err != nil {
		t.Fatalf("collision removed namespace: %v", err)
	}
	if existing.Labels["owner"] != "someone-else" {
		t.Fatalf("collision changed foreign namespace: %+v", existing.Labels)
	}
	created, err := CreateFreshNamespace(ctx, cli, "new-test")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "new-test" {
		t.Fatalf("created unexpected namespace %q", created.Name)
	}
}

type namespaceCreateFaultClient struct {
	client.Client
	create func(context.Context, client.Object, ...client.CreateOption) error
}

func (cli *namespaceCreateFaultClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return cli.create(ctx, obj, opts...)
}

func TestCreateFreshNamespaceRetriesTransientFailure(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	backend := fake.NewClientBuilder().WithScheme(s).Build()
	calls := 0
	cli := &namespaceCreateFaultClient{Client: backend, create: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
		calls++
		if calls == 1 {
			return apierrors.NewServiceUnavailable("temporary API outage")
		}
		return backend.Create(ctx, obj, opts...)
	}}
	ns, err := CreateFreshNamespace(context.Background(), cli, "retry-namespace")
	if err != nil || calls != 2 || ns.Name != "retry-namespace" {
		t.Fatalf("transient create: namespace=%q calls=%d error=%v", ns.Name, calls, err)
	}
}

func TestCreateFreshNamespaceRecoversAcceptedTimeout(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	backend := fake.NewClientBuilder().WithScheme(s).Build()
	cli := &namespaceCreateFaultClient{Client: backend, create: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
		if err := backend.Create(ctx, obj, opts...); err != nil {
			return err
		}
		return apierrors.NewTimeoutError("response was lost after creation", 1)
	}}
	ns, err := CreateFreshNamespace(context.Background(), cli, "accepted-namespace")
	if err != nil || ns.Name != "accepted-namespace" {
		t.Fatalf("accepted timeout was not recovered: %q, %v", ns.Name, err)
	}
	if err := backend.Delete(context.Background(), &ns); err != nil {
		t.Fatalf("recovered namespace cannot be cleaned up: %v", err)
	}
}

func TestCreateFreshNamespaceDoesNotAdoptForeignAfterTransientFailure(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	backend := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "occupied", Labels: map[string]string{"owner": "foreign"}}},
	).Build()
	cli := &namespaceCreateFaultClient{Client: backend, create: func(context.Context, client.Object, ...client.CreateOption) error {
		return apierrors.NewServiceUnavailable("ambiguous transport failure")
	}}
	if _, err := CreateFreshNamespace(context.Background(), cli, "occupied"); !apierrors.IsAlreadyExists(err) {
		t.Fatalf("foreign namespace was not a collision: %v", err)
	}
	var ns corev1.Namespace
	if err := backend.Get(context.Background(), client.ObjectKey{Name: "occupied"}, &ns); err != nil || ns.Labels["owner"] != "foreign" {
		t.Fatalf("foreign namespace changed: %+v, %v", ns, err)
	}
}

func TestCreateFreshNamespaceDoesNotRetryForbiddenOrIgnoreCancellation(t *testing.T) {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	backend := fake.NewClientBuilder().WithScheme(s).Build()
	calls := 0
	cli := &namespaceCreateFaultClient{Client: backend, create: func(context.Context, client.Object, ...client.CreateOption) error {
		calls++
		return apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "denied", nil)
	}}
	if _, err := CreateFreshNamespace(context.Background(), cli, "denied"); !apierrors.IsForbidden(err) || calls != 1 {
		t.Fatalf("forbidden create retried or lost its error: calls=%d, %v", calls, err)
	}
	cli.create = func(context.Context, client.Object, ...client.CreateOption) error {
		return apierrors.NewServiceUnavailable("API is down")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := CreateFreshNamespace(ctx, cli, "cancelled"); err == nil {
		t.Fatal("cancelled create returned success")
	}
}
