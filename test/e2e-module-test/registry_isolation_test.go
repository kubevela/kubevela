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

package controllers_test

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateOCIRegistryKeepsCasesSeparate(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "first"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "second"}},
	).Build()

	for _, ns := range []string{"first", "second"} {
		if err := createOCIRegistry(ctx, cli, ns); err != nil {
			t.Fatalf("create registry in %s: %v", ns, err)
		}
		var deployment appsv1.Deployment
		if err := cli.Get(ctx, client.ObjectKey{Name: "oci-registry", Namespace: ns}, &deployment); err != nil {
			t.Fatalf("registry deployment missing from %s: %v", ns, err)
		}
		var service corev1.Service
		if err := cli.Get(ctx, client.ObjectKey{Name: "oci-registry", Namespace: ns}, &service); err != nil {
			t.Fatalf("registry service missing from %s: %v", ns, err)
		}
		if service.Spec.Type != corev1.ServiceTypeNodePort || len(service.Spec.Ports) != 1 || service.Spec.Ports[0].NodePort != 0 {
			t.Fatalf("registry in %s must request a dynamically allocated NodePort: %+v", ns, service.Spec)
		}
	}

	if err := createOCIRegistry(ctx, cli, "first"); err == nil {
		t.Fatal("second registry creation must not silently adopt existing objects")
	}
}

func TestRegistryNodePortReadsTheOwningService(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "oci-registry", Namespace: "first"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{NodePort: 30111}}}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "oci-registry", Namespace: "second"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{NodePort: 30222}}}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "oci-registry", Namespace: "unallocated"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{}}}},
	).Build()
	for _, tc := range []struct {
		namespace string
		want      int32
	}{
		{namespace: "first", want: 30111},
		{namespace: "second", want: 30222},
	} {
		got, err := registryNodePort(ctx, cli, tc.namespace)
		if err != nil || got != tc.want {
			t.Errorf("registryNodePort(%q) = %d, %v; want %d", tc.namespace, got, err, tc.want)
		}
	}
	if _, err := registryNodePort(ctx, cli, "unallocated"); err == nil {
		t.Fatal("an unallocated NodePort must not be used to build a registry URL")
	}
}
