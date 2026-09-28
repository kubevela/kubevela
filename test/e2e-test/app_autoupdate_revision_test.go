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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestAutoUpdateRevisionReadyRequiresExactVersionAndType(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1beta1.SchemeBuilder.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&v1beta1.DefinitionRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "configmap-component-v1.2.0", Namespace: "tenant-a"},
			Spec:       v1beta1.DefinitionRevisionSpec{DefinitionType: common.ComponentType},
		},
	).Build()
	ctx := context.Background()
	if err := autoUpdateRevisionReady(ctx, cli, "tenant-a", "configmap-component", "1.0.0", common.ComponentType); err == nil {
		t.Fatal("a different version must not make the requested revision ready")
	}
	if err := autoUpdateRevisionReady(ctx, cli, "tenant-a", "configmap-component", "1.2.0", common.TraitType); err == nil {
		t.Fatal("a component revision must not satisfy a trait lookup")
	}
	if err := autoUpdateRevisionReady(ctx, cli, "tenant-a", "configmap-component", "1.2.0", common.ComponentType); err != nil {
		t.Fatal(err)
	}
}
