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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
