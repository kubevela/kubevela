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

package addon

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

// importWithPlacement is an _imports.cue with one import whose placement field
// is the given CUE.
func importWithPlacement(placement string) string {
	return `imports: [{
    module: "store"
    sources: [{ registry: "modules", version: "1.0.5" }]
    placement: ` + placement + `
}]`
}

// addonPolicy is one of the addon Application's rendered policies.
func addonPolicy(name, typ string, properties map[string]interface{}) v1beta1.AppPolicy {
	raw, err := json.Marshal(properties)
	Expect(err).NotTo(HaveOccurred())
	return v1beta1.AppPolicy{Name: name, Type: typ, Properties: &runtime.RawExtension{Raw: raw}}
}

// renderedPlacement renders one import and returns its placement property, or
// nil and false when the component carries none.
func renderedPlacement(place *ImportPlacement, policies []v1beta1.AppPolicy) (interface{}, bool) {
	addon := &InstallPackage{Imports: []ModuleImport{{Module: "store", Enabled: true, Placement: place}}}
	comps, err := RenderModuleComponents(addon, nil, nil, policies)
	Expect(err).NotTo(HaveOccurred())
	Expect(comps).To(HaveLen(1))
	var props map[string]interface{}
	Expect(json.Unmarshal(comps[0].Properties.Raw, &props)).To(Succeed())
	got, ok := props["placement"]
	return got, ok
}

var _ = Describe("parseModuleImports placement", func() {
	It("keeps a literal placement", func() {
		imports, err := parseModuleImports(importWithPlacement(`{ clusters: ["spoke-a", "spoke-b"] }`))
		Expect(err).NotTo(HaveOccurred())
		Expect(imports).To(HaveLen(1))
		Expect(imports[0].Placement).To(Equal(&ImportPlacement{Clusters: []string{"spoke-a", "spoke-b"}}))
	})

	It("accepts fromPolicy and leaves it for the render to resolve", func() {
		imports, err := parseModuleImports(importWithPlacement(`{ fromPolicy: "topo-runtime" }`))
		Expect(err).NotTo(HaveOccurred())
		Expect(imports[0].Placement).To(Equal(&ImportPlacement{FromPolicy: "topo-runtime"}))
	})

	It("leaves Placement nil when the entry sets none", func() {
		imports, err := parseModuleImports(`imports: [{ module: "store", sources: [{ registry: "modules" }] }]`)
		Expect(err).NotTo(HaveOccurred())
		Expect(imports[0].Placement).To(BeNil())
	})

	DescribeTable("rejects a bad placement",
		func(placement, want string) {
			_, err := parseModuleImports(importWithPlacement(placement))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("modules/_imports.cue: " + want))
		},
		Entry("fromPolicy with clusters", `{ fromPolicy: "topo-runtime", clusters: ["spoke"] }`,
			`module "store": placement.fromPolicy cannot be combined with clusters, clusterLabelSelector or allowEmpty`),
		Entry("fromPolicy with allowEmpty", `{ fromPolicy: "topo-runtime", allowEmpty: true }`,
			`module "store": placement.fromPolicy cannot be combined with clusters, clusterLabelSelector or allowEmpty`),
		Entry("clusters and a selector", `{ clusters: ["spoke"], clusterLabelSelector: { tier: "runtime" } }`,
			`module "store": placement must set clusters or clusterLabelSelector, not both`),
		Entry("an empty placement", `{}`,
			`module "store": placement must set clusters or clusterLabelSelector`),
		Entry("an empty cluster list", `{ clusters: [] }`,
			`module "store": placement clusters must name at least one cluster`),
	)
})

var _ = Describe("RenderModuleComponents placement", func() {
	It("writes a literal placement exactly", func() {
		got, ok := renderedPlacement(&ImportPlacement{Clusters: []string{"spoke-a", "spoke-b"}}, nil)
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(map[string]interface{}{"clusters": []interface{}{"spoke-a", "spoke-b"}}))

		got, ok = renderedPlacement(&ImportPlacement{ClusterLabelSelector: map[string]string{}, AllowEmpty: true}, nil)
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(map[string]interface{}{"clusterLabelSelector": map[string]interface{}{}, "allowEmpty": true}),
			"an empty selector matches every cluster and is kept")
	})

	It("copies the named topology policy's clusters and allowEmpty, never its namespace", func() {
		policies := []v1beta1.AppPolicy{
			addonPolicy("override-all", "override", map[string]interface{}{"components": []interface{}{}}),
			addonPolicy("topo-runtime", "topology", map[string]interface{}{
				"clusters": []interface{}{"spoke"}, "namespace": "addon-ns", "allowEmpty": true,
			}),
		}
		got, ok := renderedPlacement(&ImportPlacement{FromPolicy: "topo-runtime"}, policies)
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(map[string]interface{}{"clusters": []interface{}{"spoke"}, "allowEmpty": true}))
	})

	// The topology policy reads the deprecated clusterSelector spelling only
	// while the DeprecatedPolicySpec gate is on. fromPolicy reads it the same
	// way, so the module never goes to clusters the addon itself does not.
	It("reads the deprecated clusterSelector spelling while the DeprecatedPolicySpec gate is on", func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultMutableFeatureGate, features.DeprecatedPolicySpec, true)
		policies := []v1beta1.AppPolicy{
			addonPolicy("by-label", "topology", map[string]interface{}{"clusterSelector": map[string]interface{}{"tier": "runtime"}}),
		}
		got, ok := renderedPlacement(&ImportPlacement{FromPolicy: "by-label"}, policies)
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal(map[string]interface{}{"clusterLabelSelector": map[string]interface{}{"tier": "runtime"}}))
	})

	It("refuses the deprecated clusterSelector spelling while the DeprecatedPolicySpec gate is off", func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultMutableFeatureGate, features.DeprecatedPolicySpec, false)
		policies := []v1beta1.AppPolicy{
			addonPolicy("by-label", "topology", map[string]interface{}{"clusterSelector": map[string]interface{}{"tier": "runtime"}}),
		}
		addon := &InstallPackage{Imports: []ModuleImport{{Module: "store", Enabled: true, Placement: &ImportPlacement{FromPolicy: "by-label"}}}}
		_, err := RenderModuleComponents(addon, nil, nil, policies)
		Expect(err).To(MatchError(`modules/_imports.cue: module "store": placement.fromPolicy "by-label": the policy sets the deprecated clusterSelector, which is ignored unless the DeprecatedPolicySpec feature gate is on; use clusterLabelSelector`))
	})

	DescribeTable("rejects a fromPolicy that cannot be resolved",
		func(policy, want string) {
			policies := []v1beta1.AppPolicy{
				addonPolicy("override-all", "override", map[string]interface{}{"components": []interface{}{}}),
				addonPolicy("ns-only", "topology", map[string]interface{}{"namespace": "addon-ns"}),
			}
			addon := &InstallPackage{Imports: []ModuleImport{{Module: "store", Enabled: true, Placement: &ImportPlacement{FromPolicy: policy}}}}
			_, err := RenderModuleComponents(addon, nil, nil, policies)
			Expect(err).To(MatchError(want))
		},
		Entry("a missing policy", "does-not-exist",
			`modules/_imports.cue: module "store": placement.fromPolicy "does-not-exist": no such policy`),
		Entry("not a topology policy", "override-all",
			`modules/_imports.cue: module "store": placement.fromPolicy "override-all" is a override policy, not topology`),
		Entry("a policy that selects no clusters", "ns-only",
			`modules/_imports.cue: module "store": placement.fromPolicy "ns-only": placement must set clusters or clusterLabelSelector`),
	)

	It("renders an import with no placement exactly as before, even when the addon has policies", func() {
		policies := []v1beta1.AppPolicy{addonPolicy("topo-runtime", "topology", map[string]interface{}{"clusters": []interface{}{"spoke"}})}
		addon := &InstallPackage{Imports: []ModuleImport{{Module: "store", Enabled: true, Registry: "modules", Version: "1.0.5"}}}
		comps, err := RenderModuleComponents(addon, nil, nil, policies)
		Expect(err).NotTo(HaveOccurred())
		Expect(comps).To(HaveLen(1))
		Expect(string(comps[0].Properties.Raw)).To(MatchJSON(`{"module":"store","registry":"modules","version":"1.0.5"}`))
	})
})
