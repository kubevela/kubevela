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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAddonNamespaceCreationNeverAdoptsAnExistingNamespace(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existing", Labels: map[string]string{"owner": "foreign"}}},
	).Build()
	if _, err := createAddonNamespace(ctx, cli, "existing"); err == nil {
		t.Fatal("namespace collision must fail")
	}
	var existing corev1.Namespace
	if err := cli.Get(ctx, client.ObjectKey{Name: "existing"}, &existing); err != nil || existing.Labels["owner"] != "foreign" {
		t.Fatalf("collision changed a foreign namespace: %v, %+v", err, existing.Labels)
	}
	name := uniqueAddonNamespace()
	if errs := validation.IsDNS1123Label(name); len(errs) != 0 {
		t.Fatalf("invalid addon namespace %q: %v", name, errs)
	}
	if _, err := createAddonNamespace(ctx, cli, name); err != nil {
		t.Fatal(err)
	}
}
