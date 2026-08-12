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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestTraitConflictRuleMatches(t *testing.T) {
	cueTrait := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "scaler",
			Labels: map[string]string{"team": "platform"},
		},
	}
	crdTrait := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "service"},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{Name: "services.k8s.io"},
		},
	}

	testCases := []struct {
		name   string
		rule   string
		target *v1beta1.TraitDefinition
		want   bool
	}{
		{name: "wildcard", rule: "*", target: cueTrait, want: true},
		{name: "definition name match", rule: "scaler", target: cueTrait, want: true},
		{name: "definition name miss", rule: "ingress", target: cueTrait, want: false},
		{name: "crd name match", rule: "services.k8s.io", target: crdTrait, want: true},
		{name: "crd name ignored for empty reference", rule: "services.k8s.io", target: cueTrait, want: false},
		{name: "group wildcard match", rule: "*.k8s.io", target: crdTrait, want: true},
		{name: "group wildcard miss", rule: "*.networking.k8s.io", target: crdTrait, want: false},
		{name: "group wildcard ignored for empty reference", rule: "*.k8s.io", target: cueTrait, want: false},
		{name: "label selector match", rule: "labelSelector:team=platform", target: cueTrait, want: true},
		{name: "label selector miss", rule: "labelSelector:team=edge", target: cueTrait, want: false},
		{name: "invalid label selector", rule: "labelSelector:@@@", target: cueTrait, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, traitConflictRuleMatches(tc.rule, tc.target))
		})
	}
}

func TestValidateTraitConflicts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))

	conflictA := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "conflict-a", Namespace: oam.SystemDefinitionNamespace},
		Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"conflict-b"}},
	}
	conflictB := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "conflict-b", Namespace: oam.SystemDefinitionNamespace},
		Spec:       v1beta1.TraitDefinitionSpec{},
	}
	scaler := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "scaler", Namespace: oam.SystemDefinitionNamespace},
		Spec:       v1beta1.TraitDefinitionSpec{},
	}
	service := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "service", Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{Name: "services.k8s.io"},
		},
	}
	ingress := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress",
			Namespace: oam.SystemDefinitionNamespace,
			Labels:    map[string]string{"feature": "expose"},
		},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference:     common.DefinitionReference{Name: "ingresses.networking.k8s.io"},
			ConflictsWith: []string{"*.networking.k8s.io"},
		},
	}
	gateway := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: oam.SystemDefinitionNamespace},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{Name: "gateways.networking.k8s.io"},
		},
	}
	labelConflict := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "label-conflict", Namespace: oam.SystemDefinitionNamespace},
		Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"labelSelector:feature=expose"}},
	}
	// Namespaced override of conflict-a declares a conflict; the system one does not.
	nsConflictA := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "conflict-a", Namespace: "default"},
		Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"scaler"}},
	}

	baseObjects := []runtime.Object{
		conflictA.DeepCopy(), conflictB.DeepCopy(), scaler.DeepCopy(),
		service.DeepCopy(), ingress.DeepCopy(), gateway.DeepCopy(), labelConflict.DeepCopy(),
	}

	testCases := []struct {
		name               string
		extraObjects       []runtime.Object
		traits             []common.ApplicationTrait
		expectedErrorCount int
	}{
		{
			name: "unidirectional definition-name conflict is rejected",
			traits: []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "conflict-b"},
			},
			expectedErrorCount: 1,
		},
		{
			name: "non-conflicting traits are allowed",
			traits: []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "scaler"},
			},
			expectedErrorCount: 0,
		},
		{
			name: "single trait cannot conflict",
			traits: []common.ApplicationTrait{
				{Type: "conflict-a"},
			},
			expectedErrorCount: 0,
		},
		{
			name: "crd name conflict is rejected",
			extraObjects: []runtime.Object{
				func() runtime.Object {
					td := conflictA.DeepCopy()
					td.Spec.ConflictsWith = []string{"services.k8s.io"}
					return td
				}(),
			},
			traits: []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "service"},
			},
			expectedErrorCount: 1,
		},
		{
			name: "group wildcard conflict is rejected",
			traits: []common.ApplicationTrait{
				{Type: "ingress"},
				{Type: "gateway"},
			},
			expectedErrorCount: 1,
		},
		{
			name: "labelSelector conflict is rejected",
			traits: []common.ApplicationTrait{
				{Type: "label-conflict"},
				{Type: "ingress"},
			},
			expectedErrorCount: 1,
		},
		{
			name: "namespaced TraitDefinition overrides system definition",
			extraObjects: []runtime.Object{
				nsConflictA.DeepCopy(),
			},
			traits: []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "scaler"},
			},
			// System conflict-a only conflicts with conflict-b. The namespaced
			// override conflicts with scaler and must win the lookup.
			expectedErrorCount: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			objects := append([]runtime.Object{}, baseObjects...)
			if tc.name == "crd name conflict is rejected" {
				objects = []runtime.Object{
					conflictB.DeepCopy(), scaler.DeepCopy(), service.DeepCopy(),
					ingress.DeepCopy(), gateway.DeepCopy(), labelConflict.DeepCopy(),
				}
			}
			if tc.extraObjects != nil {
				objects = append(objects, tc.extraObjects...)
			}

			handler := &ValidatingHandler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build(),
			}
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:   "web",
						Type:   "webservice",
						Traits: tc.traits,
					}},
				},
			}

			errs := handler.ValidateTraitConflicts(context.Background(), app)
			assert.Equal(t, tc.expectedErrorCount, len(errs), "errs=%v", errs)
		})
	}
}

func TestValidateTraitConflictsRunsOutsideValidateComponents(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))

	conflictA := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "conflict-a", Namespace: oam.SystemDefinitionNamespace},
		Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"conflict-b"}},
	}
	conflictB := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "conflict-b", Namespace: oam.SystemDefinitionNamespace},
	}

	handler := &ValidatingHandler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(conflictA, conflictB).Build(),
	}
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name: "web",
				Type: "webservice",
				Traits: []common.ApplicationTrait{
					{Type: "conflict-a"},
					{Type: "conflict-b"},
				},
			}},
		},
	}

	// ValidateTraitConflicts itself must report the conflict even when component
	// schematic validation would be skipped under sharding.
	errs := handler.ValidateTraitConflicts(context.Background(), app)
	require.Len(t, errs, 1)
}
