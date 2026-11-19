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

package appfile

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

const statusParentTemplate = `
output: {apiVersion: "apps/v1", kind: "Deployment"}
outputs: {
	svc: {apiVersion: "v1", kind: "Service"}
	netpol: {apiVersion: "networking.k8s.io/v1", kind: "NetworkPolicy"}
}
parameter: {}
`

var _ = Describe("a child that leaves out an output its inherited status reads", func() {
	BeforeEach(func() {
		before := map[string]bool{string(features.EnableDefinitionInheritance): utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance)}
		Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.EnableDefinitionInheritance): true})).To(Succeed())
		DeferCleanup(func() { Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(before)).To(Succeed()) })
	})

	parentWith := func(status *common.Status) *v1beta1.ComponentDefinition {
		p := compDef("base", "", statusParentTemplate)
		p.Spec.Status = status
		return p
	}
	dropsSvc := func(status *common.Status) *v1beta1.ComponentDefinition {
		c := compDef("child", "base", "$super: properties: {}\n$inherit: {outputs: {svc: false}}\n")
		c.Spec.Status = status
		return c
	}
	resolve := func(child *v1beta1.ComponentDefinition, defs ...*v1beta1.ComponentDefinition) error {
		byName := map[string]*v1beta1.ComponentDefinition{}
		for _, d := range defs {
			byName[d.Name] = d
		}
		return resolveComponentChain(context.Background(), &Template{}, child, fetcherOver(byName))
	}

	DescribeTable("refuses it while the child keeps the parent's status",
		func(parent *common.Status, want string) {
			Expect(resolve(dropsSvc(nil), parentWith(parent))).To(MatchError(ContainSubstring(want)))
		},
		Entry("a health policy", &common.Status{HealthPolicy: "isHealth: context.outputs.svc.spec.clusterIP != \"\""},
			"component child leaves out outputs.svc, which the healthPolicy it inherits from base reads"),
		Entry("a status message", &common.Status{CustomStatus: "message: context.outputs.svc.spec.clusterIP"},
			"component child leaves out outputs.svc, which the customStatus it inherits from base reads"),
		Entry("details", &common.Status{Details: "ip: context.outputs.svc.spec.clusterIP"},
			"component child leaves out outputs.svc, which the details it inherits from base read"),
		Entry("an index read", &common.Status{HealthPolicy: "isHealth: context.outputs[\"svc\"] != _|_"},
			"which the healthPolicy it inherits from base reads"),
		Entry("outputs reached by an index on context", &common.Status{HealthPolicy: "isHealth: context[\"outputs\"].svc != _|_"},
			"component child leaves out outputs.svc, which the healthPolicy it inherits from base reads"),
		Entry("a read through an alias of context", &common.Status{HealthPolicy: "let c = context\nisHealth: c.outputs.svc != _|_"},
			"component child leaves out outputs.svc, which the healthPolicy it inherits from base reads"),
	)

	DescribeTable("accepts it once the child takes that concern over",
		func(parent, child *common.Status) {
			Expect(resolve(dropsSvc(child), parentWith(parent))).To(Succeed())
		},
		Entry("its own health policy", &common.Status{HealthPolicy: "isHealth: context.outputs.svc != _|_"},
			&common.Status{HealthPolicy: "$inherit: false\nisHealth: true"}),
		Entry("its own status message", &common.Status{CustomStatus: "message: context.outputs.svc.kind"},
			&common.Status{CustomStatus: "message: \"ok\""}),
		Entry("its own details", &common.Status{Details: "ip: context.outputs.svc.spec.clusterIP"},
			&common.Status{Details: "$inherit: false\nmode: \"headless\""}),
	)

	It("still refuses a health policy the child only adds to", func() {
		parent := &common.Status{HealthPolicy: "isHealth: context.outputs.svc != _|_"}
		child := &common.Status{HealthPolicy: "isHealth: true"}
		Expect(resolve(dropsSvc(child), parentWith(parent))).To(MatchError(ContainSubstring("healthPolicy it inherits")))
	})

	It("refuses it when the status reads the whole of context.outputs", func() {
		parent := &common.Status{HealthPolicy: "isHealth: len([for k, v in context.outputs if v.status == _|_ {k}]) == 0"}
		Expect(resolve(dropsSvc(nil), parentWith(parent))).To(MatchError(ContainSubstring("healthPolicy it inherits from base reads")))
	})

	It("refuses a middle level's drop that the root's status reads", func() {
		root := parentWith(&common.Status{HealthPolicy: "isHealth: context.outputs.svc != _|_"})
		middle := compDef("middle", "base", "$super: properties: {}\n$inherit: {outputs: {svc: false}}\n")
		leaf := compDef("leaf", "middle", "$super: properties: {}\n")
		Expect(resolve(leaf, root, middle)).To(MatchError(ContainSubstring(
			"component middle leaves out outputs.svc, which the healthPolicy it inherits from base reads")))
	})

	It("accepts leaving out an output no status reads", func() {
		parent := &common.Status{HealthPolicy: "isHealth: context.outputs.netpol != _|_"}
		Expect(resolve(dropsSvc(nil), parentWith(parent))).To(Succeed())
	})

	It("follows a status up through a silent middle", func() {
		root := parentWith(&common.Status{HealthPolicy: "isHealth: context.outputs.svc != _|_"})
		middle := compDef("middle", "base", "$super: properties: {}\n")
		leaf := compDef("leaf", "middle", "$super: properties: {}\n$inherit: {outputs: {svc: false}}\n")
		Expect(resolve(leaf, root, middle)).To(MatchError(ContainSubstring(
			"component leaf leaves out outputs.svc, which the healthPolicy it inherits from base reads")))
	})

	It("holds a trait's status over the trait outputs it leaves out", func() {
		base := traitDef("base-expose", "", "outputs: service: {apiVersion: \"v1\", kind: \"Service\"}\nparameter: {}\n")
		base.Spec.Status = &common.Status{HealthPolicy: "isHealth: context.outputs.service != _|_"}
		child := traitDef("own-expose", "base-expose", "$super: properties: {}\n$inherit: {outputs: {service: false}}\n")
		err := resolveTraitChain(context.Background(), &Template{}, child,
			traitFetcherOver(map[string]*v1beta1.TraitDefinition{"base-expose": base}))
		Expect(err).To(MatchError(ContainSubstring(
			"trait own-expose leaves out outputs.service, which the healthPolicy it inherits from base-expose reads")))
	})
})
