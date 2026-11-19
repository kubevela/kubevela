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
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/features"
)

var _ = Describe("a child's workload is its parent's", func() {
	BeforeEach(func() {
		before := map[string]bool{string(features.EnableDefinitionInheritance): utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableDefinitionInheritance)}
		Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(features.EnableDefinitionInheritance): true})).To(Succeed())
		DeferCleanup(func() { Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(before)).To(Succeed()) })
	})

	deployment := common.WorkloadTypeDescriptor{
		Type:       "deployments.apps",
		Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"},
	}
	statefulSet := common.WorkloadTypeDescriptor{
		Type:       "statefulsets.apps",
		Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "StatefulSet"},
	}
	autodetect := common.WorkloadTypeDescriptor{Type: types.AutoDetectWorkloadDefinition}

	level := func(name, extends string, w common.WorkloadTypeDescriptor) *v1beta1.ComponentDefinition {
		d := compDef(name, extends, "$super: properties: {}\n")
		if extends == "" {
			d = compDef(name, "", "output: {}\nparameter: {}\n")
		}
		d.Spec.Workload = w
		return d
	}
	resolve := func(child *v1beta1.ComponentDefinition, defs ...*v1beta1.ComponentDefinition) (*Template, error) {
		byName := map[string]*v1beta1.ComponentDefinition{}
		for _, d := range defs {
			byName[d.Name] = d
		}
		tmpl := &Template{}
		return tmpl, resolveComponentChain(context.Background(), tmpl, child, fetcherOver(byName))
	}

	It("refuses a child that declares a different workload", func() {
		_, err := resolve(level("sts-web", "base", statefulSet), level("base", "", deployment))
		Expect(err).To(MatchError(ContainSubstring(
			"component sts-web declares workload apps/v1 StatefulSet, but base declares apps/v1 Deployment; " +
				"leave spec.workload unset to inherit it")))
	})

	It("refuses a middle level that declares a different workload", func() {
		_, err := resolve(level("leaf", "middle", autodetect),
			level("middle", "base", statefulSet), level("base", "", deployment))
		Expect(err).To(MatchError(ContainSubstring("component middle declares workload apps/v1 StatefulSet, but base declares")))
	})

	It("refuses a partial workload that cannot be the parent's", func() {
		partial := common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "batch/v1"}}
		_, err := resolve(level("child", "base", partial), level("base", "", deployment))
		Expect(err).To(MatchError(ContainSubstring("component child declares workload batch/v1, but base declares")))
	})

	DescribeTable("accepts a child whose workload is the parent's",
		func(w common.WorkloadTypeDescriptor) {
			child := level("child", "base", w)
			tmpl, err := resolve(child, level("base", "", deployment))
			Expect(err).NotTo(HaveOccurred())
			Expect(child.Spec.Workload.Definition).To(Equal(deployment.Definition))
			Expect(tmpl.Reference.Definition).To(Equal(deployment.Definition))
		},
		Entry("left unset", common.WorkloadTypeDescriptor{}),
		Entry("autodetect", autodetect),
		Entry("the same, stated again", deployment),
		Entry("the same, by type only", common.WorkloadTypeDescriptor{Type: "deployments.apps"}),
		Entry("the same apiVersion, kind left out", common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "apps/v1"}}),
	)

	It("accepts a child naming by type a workload its parent names by kind, and keeps its type", func() {
		gvkOnly := common.WorkloadTypeDescriptor{Definition: deployment.Definition}
		child := level("child", "base", common.WorkloadTypeDescriptor{Type: "deployments.apps"})
		_, err := resolve(child, level("base", "", gvkOnly))
		Expect(err).NotTo(HaveOccurred())
		Expect(child.Spec.Workload).To(Equal(deployment))
	})

	// The parent names its workload by kind alone, as one the webhook has not
	// filled a type in on does. A child's type is still compared against it,
	// not overwritten by it.
	DescribeTable("refuses a child naming by type a workload other than the one its parent names by kind",
		func(w common.WorkloadTypeDescriptor, want string) {
			gvkOnly := common.WorkloadTypeDescriptor{Definition: deployment.Definition}
			child := level("child", "base", w)
			_, err := resolve(child, level("base", "", gvkOnly))
			Expect(err).To(MatchError(ContainSubstring(want)))
			Expect(child.Spec.Workload).To(Equal(w))
		},
		Entry("another kind in the same group", common.WorkloadTypeDescriptor{Type: "statefulsets.apps"},
			"component child declares workload statefulsets.apps, but base declares apps/v1 Deployment"),
		Entry("the same resource in another group", common.WorkloadTypeDescriptor{Type: "deployments.extensions.example.io"},
			"component child declares workload deployments.extensions.example.io, but base declares apps/v1 Deployment"),
	)

	It("refuses a child naming by type a group other than its parent's apiVersion", func() {
		apiOnly := common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "apps/v1"}}
		_, err := resolve(level("child", "base", common.WorkloadTypeDescriptor{Type: "cronjobs.batch"}), level("base", "", apiOnly))
		Expect(err).To(MatchError(ContainSubstring("component child declares workload cronjobs.batch, but base declares apps/v1")))
	})

	It("matches a core-group type against its kind", func() {
		pod := common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "Pod"}}
		_, err := resolve(level("child", "base", common.WorkloadTypeDescriptor{Type: "pods"}), level("base", "", pod))
		Expect(err).NotTo(HaveOccurred())
	})

	It("passes the root's workload through a silent middle", func() {
		child := level("leaf", "middle", autodetect)
		_, err := resolve(child, level("middle", "base", autodetect), level("base", "", deployment))
		Expect(err).NotTo(HaveOccurred())
		Expect(child.Spec.Workload).To(Equal(deployment))
	})

	It("keeps a child's workload when nothing above it declares one", func() {
		child := level("child", "base", statefulSet)
		_, err := resolve(child, level("base", "", autodetect))
		Expect(err).NotTo(HaveOccurred())
		Expect(child.Spec.Workload).To(Equal(statefulSet))
	})

	// The root declares nothing, but its template writes what it renders. No
	// level can change output.kind, so a workload below it that says otherwise
	// is a wrong label.
	Context("when the root declares no workload but renders a fixed kind", func() {
		rendersDeployment := func(name string) *v1beta1.ComponentDefinition {
			d := compDef(name, "", "output: {\n\tapiVersion: \"apps/v1\"\n\tkind: \"Deployment\"\n}\nparameter: {}\n")
			d.Spec.Workload = autodetect
			return d
		}

		It("refuses a child that declares a different workload", func() {
			_, err := resolve(level("f-auto-sts", "f-auto", statefulSet), rendersDeployment("f-auto"))
			Expect(err).To(MatchError(
				"component f-auto-sts declares workload apps/v1 StatefulSet, but f-auto renders apps/v1 Deployment; " +
					"leave spec.workload unset to inherit it"))
		})

		DescribeTable("accepts a child whose workload is what the root renders",
			func(w common.WorkloadTypeDescriptor) {
				_, err := resolve(level("child", "f-auto", w), rendersDeployment("f-auto"))
				Expect(err).NotTo(HaveOccurred())
			},
			Entry("the same GVK", common.WorkloadTypeDescriptor{Definition: deployment.Definition}),
			Entry("the same, by type only", common.WorkloadTypeDescriptor{Type: "deployments.apps"}),
			Entry("left unset", common.WorkloadTypeDescriptor{}),
			Entry("autodetect", autodetect),
		)

		It("accepts a child when the root's kind comes from a parameter", func() {
			root := compDef("f-param", "", "output: {\n\tapiVersion: \"apps/v1\"\n\tkind: parameter.kind\n}\nparameter: kind: string\n")
			root.Spec.Workload = autodetect
			_, err := resolve(level("child", "f-param", statefulSet), root)
			Expect(err).NotTo(HaveOccurred())
		})

		It("refuses a leaf through a silent middle, naming the root", func() {
			_, err := resolve(level("leaf", "middle", statefulSet),
				level("middle", "f-auto", autodetect), rendersDeployment("f-auto"))
			Expect(err).To(MatchError(
				"component leaf declares workload apps/v1 StatefulSet, but f-auto renders apps/v1 Deployment; " +
					"leave spec.workload unset to inherit it"))
		})
	})

	// Three levels, and the leaf states a workload of its own. It renders the
	// root's output with more on it, so a workload other than the root's could
	// only ever be a wrong label, and is refused.
	It("refuses a leaf's own workload when the root declares another", func() {
		leaf := cdLevel("leaf", "mid", common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}})
		mid := cdLevel("mid", "root", common.WorkloadTypeDescriptor{})
		root := cdLevel("root", "", common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{APIVersion: "batch/v1", Kind: "CronJob"}})

		err := resolveComponentChain(context.Background(), &Template{Reference: leaf.Spec.Workload}, leaf, fetcherFor(mid, root))
		Expect(err).To(MatchError(ContainSubstring("component leaf declares workload apps/v1 Deployment, but root declares batch/v1 CronJob")))
	})
})
