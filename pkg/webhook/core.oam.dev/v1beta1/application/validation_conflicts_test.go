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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Trait conflict validation", func() {
	Describe("traitConflictRuleMatches", func() {
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
		nestedGroupTrait := &v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress"},
			Spec: v1beta1.TraitDefinitionSpec{
				Reference: common.DefinitionReference{Name: "ingresses.networking.k8s.io"},
			},
		}

		DescribeTable("rule matching",
			func(rule string, target *v1beta1.TraitDefinition, want bool) {
				Expect(traitConflictRuleMatches(rule, target)).To(Equal(want))
			},
			Entry("wildcard", "*", cueTrait, true),
			Entry("definition name match", "scaler", cueTrait, true),
			Entry("definition name miss", "ingress", cueTrait, false),
			Entry("crd name match", "services.k8s.io", crdTrait, true),
			Entry("crd name ignored for empty reference", "services.k8s.io", cueTrait, false),
			Entry("group wildcard match", "*.k8s.io", crdTrait, true),
			Entry("group wildcard does not match nested group suffix", "*.k8s.io", nestedGroupTrait, false),
			Entry("group wildcard miss", "*.networking.k8s.io", crdTrait, false),
			Entry("group wildcard ignored for empty reference", "*.k8s.io", cueTrait, false),
			Entry("label selector match", "labelSelector:team=platform", cueTrait, true),
			Entry("label selector miss", "labelSelector:team=edge", cueTrait, false),
			Entry("invalid label selector", "labelSelector:@@@", cueTrait, false),
		)
	})

	Describe("ValidateTraitConflicts", func() {
		var (
			scheme        *runtime.Scheme
			conflictA     *v1beta1.TraitDefinition
			conflictB     *v1beta1.TraitDefinition
			scaler        *v1beta1.TraitDefinition
			service       *v1beta1.TraitDefinition
			ingress       *v1beta1.TraitDefinition
			gateway       *v1beta1.TraitDefinition
			labelConflict *v1beta1.TraitDefinition
			nsConflictA   *v1beta1.TraitDefinition
			baseObjects   []runtime.Object
		)

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(v1beta1.AddToScheme(scheme)).To(Succeed())

			conflictA = &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "conflict-a", Namespace: oam.SystemDefinitionNamespace},
				Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"conflict-b"}},
			}
			conflictB = &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "conflict-b", Namespace: oam.SystemDefinitionNamespace},
				Spec:       v1beta1.TraitDefinitionSpec{},
			}
			scaler = &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "scaler", Namespace: oam.SystemDefinitionNamespace},
				Spec:       v1beta1.TraitDefinitionSpec{},
			}
			service = &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "service", Namespace: oam.SystemDefinitionNamespace},
				Spec: v1beta1.TraitDefinitionSpec{
					Reference: common.DefinitionReference{Name: "services.k8s.io"},
				},
			}
			ingress = &v1beta1.TraitDefinition{
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
			gateway = &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: oam.SystemDefinitionNamespace},
				Spec: v1beta1.TraitDefinitionSpec{
					Reference: common.DefinitionReference{Name: "gateways.networking.k8s.io"},
				},
			}
			labelConflict = &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "label-conflict", Namespace: oam.SystemDefinitionNamespace},
				Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"labelSelector:feature=expose"}},
			}
			nsConflictA = &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "conflict-a", Namespace: "default"},
				Spec:       v1beta1.TraitDefinitionSpec{ConflictsWith: []string{"scaler"}},
			}

			baseObjects = []runtime.Object{
				conflictA.DeepCopy(), conflictB.DeepCopy(), scaler.DeepCopy(),
				service.DeepCopy(), ingress.DeepCopy(), gateway.DeepCopy(), labelConflict.DeepCopy(),
			}
		})

		validateWith := func(objects []runtime.Object, traits []common.ApplicationTrait) int {
			handler := &ValidatingHandler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build(),
			}
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{
						Name:   "web",
						Type:   "webservice",
						Traits: traits,
					}},
				},
			}
			return len(handler.ValidateTraitConflicts(context.Background(), app))
		}

		It("rejects unidirectional definition-name conflict", func() {
			Expect(validateWith(baseObjects, []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "conflict-b"},
			})).To(Equal(1))
		})

		It("allows non-conflicting traits", func() {
			Expect(validateWith(baseObjects, []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "scaler"},
			})).To(Equal(0))
		})

		It("allows a single trait", func() {
			Expect(validateWith(baseObjects, []common.ApplicationTrait{
				{Type: "conflict-a"},
			})).To(Equal(0))
		})

		It("rejects crd name conflict", func() {
			objects := make([]runtime.Object, 0, len(baseObjects))
			for _, o := range baseObjects {
				if td, ok := o.(*v1beta1.TraitDefinition); ok && td.Name == "conflict-a" {
					td = td.DeepCopy()
					td.Spec.ConflictsWith = []string{"services.k8s.io"}
					objects = append(objects, td)
					continue
				}
				objects = append(objects, o.DeepCopyObject())
			}
			Expect(validateWith(objects, []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "service"},
			})).To(Equal(1))
		})

		It("rejects group wildcard conflict", func() {
			Expect(validateWith(baseObjects, []common.ApplicationTrait{
				{Type: "ingress"},
				{Type: "gateway"},
			})).To(Equal(1))
		})

		It("rejects labelSelector conflict", func() {
			Expect(validateWith(baseObjects, []common.ApplicationTrait{
				{Type: "label-conflict"},
				{Type: "ingress"},
			})).To(Equal(1))
		})

		It("prefers namespaced TraitDefinition over system definition", func() {
			objects := append([]runtime.Object{}, baseObjects...)
			objects = append(objects, nsConflictA.DeepCopy())
			// System conflict-a only conflicts with conflict-b. The namespaced
			// override conflicts with scaler and must win the lookup.
			Expect(validateWith(objects, []common.ApplicationTrait{
				{Type: "conflict-a"},
				{Type: "scaler"},
			})).To(Equal(1))
		})
	})
})
