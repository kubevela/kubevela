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
	)

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
})
