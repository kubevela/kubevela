/*
Copyright 2025 The KubeVela Authors.

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

package defkit_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// restrictionsOf parses generated CUE the way apply-module does and decodes
// spec.restrictions.
func restrictionsOf(cueStr string) *common.DefinitionRestrictions {
	spec := fromCUE(cueStr)
	raw, ok := spec["restrictions"]
	if !ok {
		return nil
	}
	bs, err := json.Marshal(raw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	r := &common.DefinitionRestrictions{}
	ExpectWithOffset(1, json.Unmarshal(bs, r)).To(Succeed())
	return r
}

var _ = Describe("Definition restrictions", func() {
	tenants := defkit.MatchLabels(map[string]string{"tenant": "true"})

	component := func() *defkit.ComponentDefinition {
		return defkit.NewComponent("c").Template(func(tpl *defkit.Template) {
			tpl.Output(defkit.NewResource("v1", "ConfigMap"))
		})
	}
	trait := func() *defkit.TraitDefinition {
		return defkit.NewTrait("t").Template(func(tpl *defkit.Template) {
			tpl.Patch().Set("metadata.labels.a", defkit.Lit("b"))
		})
	}

	It("writes nothing when no restriction is set", func() {
		Expect(restrictionsOf(component().ToCue())).To(BeNil())
	})

	DescribeTable("namespace restrictions reach spec.restrictions on every kind",
		func(def interface{ ToCue() string }) {
			Expect(restrictionsOf(def.ToCue())).To(Equal(&common.DefinitionRestrictions{
				Namespaces:        []string{"vela-system", "tenant-*"},
				NamespaceSelector: tenants,
			}))
		},
		Entry("component", component().
			RestrictNamespaces("vela-system", "tenant-*").
			RestrictNamespaceSelector(tenants)),
		Entry("trait", trait().
			RestrictNamespaces("vela-system", "tenant-*").
			RestrictNamespaceSelector(tenants)),
		Entry("trait with a template block", defkit.NewTrait("t").
			TemplateBlock("patch: metadata: labels: a: \"b\"\nparameter: {}").
			RestrictNamespaces("vela-system", "tenant-*").
			RestrictNamespaceSelector(tenants)),
		Entry("policy", defkit.NewPolicy("p").
			Template(func(tpl *defkit.PolicyTemplate) { tpl.Set("enabled", defkit.Lit(true)) }).
			RestrictNamespaces("vela-system", "tenant-*").
			RestrictNamespaceSelector(tenants)),
		Entry("workflow step", defkit.NewWorkflowStep("w").
			Template(func(tpl *defkit.WorkflowStepTemplate) { tpl.Set("ok", defkit.Lit(true)) }).
			RestrictNamespaces("vela-system", "tenant-*").
			RestrictNamespaceSelector(tenants)),
	)

	quota := []*defkit.NamespaceQuota{
		defkit.QuotaForSelector(defkit.MatchLabels(map[string]string{"tier": "gold"})).Warn(16).Limit(20),
		defkit.QuotaFor("tenant-dev-*").Limit(3),
		defkit.DefaultQuota().Warn(5),
	}
	wantQuota := []common.NamespaceQuota{
		{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "gold"}}, Warn: ptr.To[int32](16), Limit: ptr.To[int32](20)},
		{Namespaces: []string{"tenant-dev-*"}, Limit: ptr.To[int32](3)},
		{Warn: ptr.To[int32](5)},
	}

	DescribeTable("quota reaches spec.restrictions.quota in order",
		func(def interface{ ToCue() string }) {
			Expect(restrictionsOf(def.ToCue())).To(Equal(&common.DefinitionRestrictions{Quota: wantQuota}))
		},
		Entry("component", component().Quota(quota...)),
		Entry("trait", trait().Quota(quota...)),
	)

	It("writes restrictions into ToYAML's spec too", func() {
		bs, err := component().RestrictNamespaces("tenant-*").Quota(defkit.DefaultQuota().Limit(2)).ToYAML()
		Expect(err).NotTo(HaveOccurred())
		var cr struct {
			Spec struct {
				Restrictions *common.DefinitionRestrictions `json:"restrictions"`
			} `json:"spec"`
		}
		Expect(yaml.Unmarshal(bs, &cr)).To(Succeed())
		Expect(cr.Spec.Restrictions).To(Equal(&common.DefinitionRestrictions{
			Namespaces: []string{"tenant-*"},
			Quota:      []common.NamespaceQuota{{Limit: ptr.To[int32](2)}},
		}))
	})

	DescribeTable("Register refuses restrictions admission would refuse",
		func(def defkit.Definition, want string) {
			Expect(func() { defkit.Register(def) }).To(PanicWith(ContainSubstring(want)))
		},
		Entry("a bad glob", component().RestrictNamespaces("tenant-["), "not a valid pattern"),
		Entry("a default quota before the end", component().Quota(
			defkit.DefaultQuota().Limit(1), defkit.QuotaFor("a").Limit(2)), "could never apply"),
		Entry("a quota entry with neither warn nor limit", trait().Quota(defkit.QuotaFor("a")), "neither warn nor limit"),
		Entry("warn above limit", trait().Quota(defkit.DefaultQuota().Warn(3).Limit(2)), "above the limit"),
	)
})
