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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("PolicyDefinition", func() {

	Context("Basic Builder Methods", func() {
		It("should create policy with name", func() {
			p := defkit.NewPolicy("topology")
			Expect(p.DefName()).To(Equal("topology"))
			Expect(p.DefType()).To(Equal(defkit.DefinitionTypePolicy))
		})

		It("should set description", func() {
			p := defkit.NewPolicy("topology").
				Description("Deployment topology policy")
			Expect(p.GetDescription()).To(Equal("Deployment topology policy"))
		})

		It("should add single parameter with Param", func() {
			p := defkit.NewPolicy("topology").
				Param(defkit.StringList("clusters"))
			Expect(p.GetParams()).To(HaveLen(1))
			Expect(p.GetParams()[0].Name()).To(Equal("clusters"))
		})

		It("should add multiple parameters with Params", func() {
			p := defkit.NewPolicy("topology").
				Params(
					defkit.StringList("clusters"),
					defkit.Bool("allowEmpty"),
					defkit.String("namespace"),
				)
			Expect(p.GetParams()).To(HaveLen(3))
		})

		It("should chain Param calls", func() {
			p := defkit.NewPolicy("topology").
				Param(defkit.StringList("clusters")).
				Param(defkit.Bool("allowEmpty")).
				Param(defkit.String("namespace"))
			Expect(p.GetParams()).To(HaveLen(3))
		})
	})

	Context("Template method", func() {
		It("should set template function", func() {
			p := defkit.NewPolicy("custom").
				Template(func(tpl *defkit.PolicyTemplate) {
					tpl.SetField("clusters", defkit.ParamRef("clusters"))
				})
			Expect(p).NotTo(BeNil())
		})
	})

	Context("Helper method", func() {
		It("should add helper definition with param", func() {
			selectorParam := defkit.Struct("selector").Fields(
				defkit.Field("name", defkit.ParamTypeString),
				defkit.Field("namespace", defkit.ParamTypeString),
			)
			p := defkit.NewPolicy("override").
				Helper("RuleSelector", selectorParam)
			helpers := p.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			Expect(helpers[0].GetName()).To(Equal("RuleSelector"))
			Expect(helpers[0].HasParam()).To(BeTrue())
		})

		It("should chain multiple Helper calls", func() {
			p := defkit.NewPolicy("override").
				Helper("Selector", defkit.Struct("sel")).
				Helper("Override", defkit.Struct("ovr"))
			Expect(p.GetHelperDefinitions()).To(HaveLen(2))
		})
	})

	Context("CustomStatus method", func() {
		It("should set custom status expression", func() {
			p := defkit.NewPolicy("topology").
				CustomStatus("message: \"Policy applied\"")
			Expect(p.GetCustomStatus()).To(Equal("message: \"Policy applied\""))
		})
	})

	Context("HealthPolicy method", func() {
		It("should set health policy expression", func() {
			p := defkit.NewPolicy("topology").
				HealthPolicy("isHealth: true")
			Expect(p.GetHealthPolicy()).To(Equal("isHealth: true"))
		})
	})

	Context("HealthPolicyExpr method", func() {
		It("should set health policy from HealthExpression", func() {
			h := defkit.Health()
			p := defkit.NewPolicy("topology").
				HealthPolicyExpr(h.Condition("Ready").IsTrue())
			Expect(p.GetHealthPolicy()).NotTo(BeEmpty())
		})
	})

	Context("WithImports method", func() {
		It("should add imports to policy", func() {
			p := defkit.NewPolicy("custom").
				WithImports("strings", "list")
			Expect(p.GetImports()).To(ConsistOf("strings", "list"))
		})

		It("should accumulate imports with multiple calls", func() {
			p := defkit.NewPolicy("custom").
				WithImports("strings").
				WithImports("list", "math")
			Expect(p.GetImports()).To(HaveLen(3))
		})
	})

	Context("RawCUE method", func() {
		It("should set raw CUE and bypass generation", func() {
			rawCUE := `"topology": {
	type: "policy"
	description: "Raw policy"
}
template: {
	parameter: clusters?: [...string]
}`
			p := defkit.NewPolicy("topology").RawCUE(rawCUE)
			Expect(p.HasRawCUE()).To(BeTrue())
			Expect(p.ToCue()).To(Equal(rawCUE))
		})
	})

	Context("ToCue Generation", func() {
		It("should generate complete CUE definition", func() {
			p := defkit.NewPolicy("topology").
				Description("Deployment topology").
				Params(
					defkit.StringList("clusters"),
					defkit.Bool("allowEmpty").Default(false),
				)

			cue := p.ToCue()

			Expect(cue).To(ContainSubstring(`type: "policy"`))
			Expect(cue).To(ContainSubstring(`description: "Deployment topology"`))
			Expect(cue).To(ContainSubstring("template:"))
			Expect(cue).To(ContainSubstring("parameter:"))
			Expect(cue).To(ContainSubstring("clusters"))
			Expect(cue).To(ContainSubstring("allowEmpty"))
		})

		It("should include imports in CUE output", func() {
			p := defkit.NewPolicy("custom").
				Description("Custom policy").
				WithImports("strings", "list")

			cue := p.ToCue()

			Expect(cue).To(ContainSubstring(`import (`))
			Expect(cue).To(ContainSubstring(`"strings"`))
			Expect(cue).To(ContainSubstring(`"list"`))
		})
	})

	Context("ToYAML Generation", func() {
		It("should generate valid YAML manifest", func() {
			p := defkit.NewPolicy("topology").
				Description("Deployment topology").
				Params(defkit.StringList("clusters"))

			yamlBytes, err := p.ToYAML()
			Expect(err).NotTo(HaveOccurred())

			yaml := string(yamlBytes)
			Expect(yaml).To(ContainSubstring("kind: PolicyDefinition"))
			Expect(yaml).To(ContainSubstring("name: topology"))
		})
	})

	Context("PolicyTemplate", func() {
		It("should create a new policy template", func() {
			tpl := defkit.NewPolicyTemplate()
			Expect(tpl).NotTo(BeNil())
		})

		It("should set field on template", func() {
			tpl := defkit.NewPolicyTemplate()
			tpl.SetField("clusters", defkit.ParamRef("clusters"))
			fields := tpl.GetComputedFields()
			Expect(fields).To(HaveKey("clusters"))
		})

		It("should set multiple fields", func() {
			tpl := defkit.NewPolicyTemplate()
			tpl.SetField("clusters", defkit.ParamRef("clusters"))
			tpl.SetField("namespace", defkit.ParamRef("namespace"))
			fields := tpl.GetComputedFields()
			Expect(fields).To(HaveLen(2))
		})
	})

	Context("Registry", func() {
		BeforeEach(func() {
			defkit.Clear()
		})

		AfterEach(func() {
			defkit.Clear()
		})

		It("should register policies", func() {
			policy1 := defkit.NewPolicy("topology").Description("Topology")
			policy2 := defkit.NewPolicy("override").Description("Override")
			comp := defkit.NewComponent("webservice").Description("Component")

			defkit.Register(policy1)
			defkit.Register(policy2)
			defkit.Register(comp)

			Expect(defkit.Count()).To(Equal(3))
			Expect(defkit.Policies()).To(HaveLen(2))
		})
	})
})
