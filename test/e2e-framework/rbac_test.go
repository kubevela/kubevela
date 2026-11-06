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

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCoreSuiteRBACCleanupOwnsOnlyItsRun(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	if err := rbacv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "oam-example-com"}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "oam-role-binding"}},
	).Build()

	first, err := installCoreSuiteRBAC(ctx, cli, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := installCoreSuiteRBAC(ctx, cli, "second")
	if err != nil {
		t.Fatal(err)
	}
	if first.roleName == second.roleName || first.bindingName == second.bindingName {
		t.Fatal("two runs reused cluster-scoped RBAC names")
	}
	if err := first.cleanup(ctx, cli); err != nil {
		t.Fatal(err)
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: first.roleName}, &rbacv1.ClusterRole{}); !apierrors.IsNotFound(err) {
		t.Errorf("cleanup did not remove its ClusterRole: %v", err)
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: first.bindingName}, &rbacv1.ClusterRoleBinding{}); !apierrors.IsNotFound(err) {
		t.Errorf("cleanup did not remove its ClusterRoleBinding: %v", err)
	}
	for _, name := range []string{"oam-example-com", second.roleName} {
		if err := cli.Get(ctx, client.ObjectKey{Name: name}, &rbacv1.ClusterRole{}); err != nil {
			t.Errorf("cleanup removed unrelated ClusterRole %s: %v", name, err)
		}
	}
	for _, name := range []string{"oam-role-binding", second.bindingName} {
		if err := cli.Get(ctx, client.ObjectKey{Name: name}, &rbacv1.ClusterRoleBinding{}); err != nil {
			t.Errorf("cleanup removed unrelated ClusterRoleBinding %s: %v", name, err)
		}
	}
	if _, err := installCoreSuiteRBAC(ctx, cli, "second"); err == nil {
		t.Fatal("a collision must fail, not adopt an existing run's RBAC")
	}

	// Same generated names, but now owned by another run: cleanup must refuse
	// rather than delete by name.
	role := &rbacv1.ClusterRole{}
	if err := cli.Get(ctx, client.ObjectKey{Name: second.roleName}, role); err != nil {
		t.Fatal(err)
	}
	role.Labels[coreSuiteOwnerLabel] = "another-run"
	if err := cli.Update(ctx, role); err != nil {
		t.Fatal(err)
	}
	binding := &rbacv1.ClusterRoleBinding{}
	if err := cli.Get(ctx, client.ObjectKey{Name: second.bindingName}, binding); err != nil {
		t.Fatal(err)
	}
	binding.Labels[coreSuiteOwnerLabel] = "another-run"
	if err := cli.Update(ctx, binding); err != nil {
		t.Fatal(err)
	}
	if err := second.cleanup(ctx, cli); err == nil {
		t.Fatal("cleanup must refuse RBAC owned by another run")
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: second.roleName}, &rbacv1.ClusterRole{}); err != nil {
		t.Errorf("cleanup removed a ClusterRole owned by another run: %v", err)
	}
	if err := cli.Get(ctx, client.ObjectKey{Name: second.bindingName}, &rbacv1.ClusterRoleBinding{}); err != nil {
		t.Errorf("cleanup removed a ClusterRoleBinding owned by another run: %v", err)
	}
}
