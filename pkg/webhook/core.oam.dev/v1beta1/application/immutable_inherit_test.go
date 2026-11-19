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
	"encoding/json"

	"cuelang.org/go/cue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

// immutableParent fixes its storage class once set.
const immutableParent = `
output: {
	apiVersion: "v1"
	kind:       "PersistentVolumeClaim"
	spec: {
		storageClassName: parameter.storageClass
		resources: requests: storage: parameter.size
	}
}
parameter: {
	// +immutable
	storageClass: string
	size:         *"1Gi" | string
}
`

var _ = Describe("+immutable through extends", func() {
	var handler *ValidatingHandler

	BeforeEach(func() {
		before := map[string]bool{string(features.EnableDefinitionInheritance): utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance)}
		Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.EnableDefinitionInheritance): true})).To(Succeed())
		DeferCleanup(func() { Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(before)).To(Succeed()) })

		scheme := runtime.NewScheme()
		Expect(v1beta1.AddToScheme(scheme)).To(Succeed())
		def := func(name, extends, template string) *v1beta1.ComponentDefinition {
			return &v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
				Spec: v1beta1.ComponentDefinitionSpec{
					Extends:   extends,
					Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
				},
			}
		}
		traitDef := func(name, extends, template string) *v1beta1.TraitDefinition {
			return &v1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
				Spec: v1beta1.TraitDefinitionSpec{
					Extends:   extends,
					Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
				},
			}
		}
		handler = &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			def("volume", "", immutableParent),
			def("forwarding-volume", "volume", "$super: properties: parameter\nparameter: {storageClass: string, size: *\"1Gi\" | string}\n"),
			def("tiered-volume", "volume", "$super: properties: {storageClass: parameter.tier, size: parameter.size}\nparameter: {tier: string, size: *\"1Gi\" | string}\n"),
			def("fixed-volume", "volume", "$super: properties: {storageClass: \"standard\", size: parameter.size}\nparameter: size: *\"1Gi\" | string\n"),
			def("replicated", "", "output: {kind: \"Deployment\", spec: replicas: parameter.replicas}\nparameter: {\n\t// +immutable\n\treplicas: *1 | int\n}\n"),
			def("replicated-child", "replicated", "$super: properties: {replicas: parameter.replicas}\nparameter: replicas: *1 | int\n"),
			def("tiered-leaf", "tiered-volume", "$super: properties: {tier: parameter.tier}\nparameter: tier: \"fast\" | \"slow\"\n"),
			traitDef("zone", "", "patch: metadata: labels: zone: parameter.zone\nparameter: {\n\t// +immutable\n\tzone: string\n}\n"),
			traitDef("region-zone", "zone", "$super: properties: {zone: parameter.region + \"-a\"}\nparameter: region: string\n"),
		).Build()}
	})

	app := func(typ string, props map[string]any) *v1beta1.Application {
		raw, err := json.Marshal(props)
		Expect(err).NotTo(HaveOccurred())
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
				{Name: "data", Type: typ, Properties: &runtime.RawExtension{Raw: raw}},
			}},
		}
	}

	It("refuses changing the parent's immutable parameter through a child that forwards it", func() {
		errs := handler.ValidateImmutableFields(context.Background(),
			app("forwarding-volume", map[string]any{"storageClass": "slow"}),
			app("forwarding-volume", map[string]any{"storageClass": "fast"}))
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Detail).To(ContainSubstring(
			`volume's immutable parameter storageClass cannot be changed (current: "fast", new: "slow")`))
	})

	It("refuses changing it through a child parameter of another name", func() {
		errs := handler.ValidateImmutableFields(context.Background(),
			app("tiered-volume", map[string]any{"tier": "slow"}),
			app("tiered-volume", map[string]any{"tier": "fast"}))
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Detail).To(ContainSubstring("volume's immutable parameter storageClass cannot be changed"))
	})

	It("accepts a change that does not reach the immutable parameter", func() {
		errs := handler.ValidateImmutableFields(context.Background(),
			app("tiered-volume", map[string]any{"tier": "fast", "size": "2Gi"}),
			app("tiered-volume", map[string]any{"tier": "fast"}))
		Expect(errs).To(BeEmpty())
	})

	It("accepts a child that fixes the immutable parameter itself", func() {
		errs := handler.ValidateImmutableFields(context.Background(),
			app("fixed-volume", map[string]any{"size": "2Gi"}),
			app("fixed-volume", map[string]any{"size": "1Gi"}))
		Expect(errs).To(BeEmpty())
	})

	It("still lets force-param-mutations through", func() {
		next := app("tiered-volume", map[string]any{"tier": "slow"})
		next.Annotations = map[string]string{"app.oam.dev/force-param-mutations": "true"}
		Expect(handler.ValidateImmutableFields(context.Background(), next,
			app("tiered-volume", map[string]any{"tier": "fast"}))).To(BeEmpty())
	})

	It("lets a parameter the parent only had as a default be set for the first time", func() {
		errs := handler.ValidateImmutableFields(context.Background(),
			app("replicated-child", map[string]any{"replicas": 3}),
			app("replicated-child", map[string]any{}))
		Expect(errs).To(BeEmpty())
	})

	It("holds the root's immutable parameter through a middle level", func() {
		errs := handler.ValidateImmutableFields(context.Background(),
			app("tiered-leaf", map[string]any{"tier": "slow"}),
			app("tiered-leaf", map[string]any{"tier": "fast"}))
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Detail).To(ContainSubstring("volume's immutable parameter storageClass cannot be changed"))
	})

	It("holds a parent trait's immutable parameter through a child trait", func() {
		withTrait := func(region string) *v1beta1.Application {
			a := app("fixed-volume", map[string]any{})
			raw, err := json.Marshal(map[string]any{"region": region})
			Expect(err).NotTo(HaveOccurred())
			a.Spec.Components[0].Traits = []common.ApplicationTrait{{Type: "region-zone", Properties: &runtime.RawExtension{Raw: raw}}}
			return a
		}
		errs := handler.ValidateImmutableFields(context.Background(), withTrait("eu"), withTrait("us"))
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Detail).To(ContainSubstring(`zone's immutable parameter zone cannot be changed (current: "us-a", new: "eu-a")`))
	})

	Context("when the properties did not change", func() {
		var compiles int

		BeforeEach(func() {
			compiles = 0
			orig := ancestorCompiler
			ancestorCompiler = func(ctx context.Context, src string) (cue.Value, error) {
				compiles++
				return orig(ctx, src)
			}
			DeferCleanup(func() { ancestorCompiler = orig })
		})

		withRaw := func(typ, raw string) *v1beta1.Application {
			a := app(typ, map[string]any{})
			a.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(raw)}
			return a
		}

		It("does not evaluate the chain", func() {
			errs := handler.ValidateImmutableFields(context.Background(),
				withRaw("tiered-leaf", `{"tier":"fast","size":"2Gi"}`),
				withRaw("tiered-leaf", `{"size": "2Gi", "tier": "fast"}`))
			Expect(errs).To(BeEmpty())
			Expect(compiles).To(BeZero())
		})

		It("tells apart integers that would round to the same float64", func() {
			Expect(sameProperties(&runtime.RawExtension{Raw: []byte(`{"n":9007199254740993}`)},
				&runtime.RawExtension{Raw: []byte(`{"n":9007199254740992}`)})).To(BeFalse())
			Expect(sameProperties(&runtime.RawExtension{Raw: []byte(`{"n":9007199254740993}`)},
				&runtime.RawExtension{Raw: []byte(`{ "n": 9007199254740993 }`)})).To(BeTrue())
		})

		It("still evaluates it once they do change", func() {
			errs := handler.ValidateImmutableFields(context.Background(),
				withRaw("tiered-leaf", `{"tier":"slow"}`),
				withRaw("tiered-leaf", `{"tier":"fast"}`))
			Expect(errs).To(HaveLen(1))
			Expect(compiles).To(BeNumerically(">", 0))
		})
	})
})
