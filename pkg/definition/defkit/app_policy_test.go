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
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// policySpec parses generated CUE the way apply-module does, strictly into the
// typed PolicyDefinitionSpec.
func policySpec(cueStr string) v1beta1.PolicyDefinitionSpec {
	bs, err := json.Marshal(fromCUE(cueStr))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	var spec v1beta1.PolicyDefinitionSpec
	ExpectWithOffset(1, json.Unmarshal(bs, &spec)).To(Succeed())
	return spec
}

// defkitTemplateOnly returns the body of the definition's template block, which
// the controller compiles together with the context and parameter.
func defkitTemplateOnly(cueStr string) string {
	start := strings.Index(cueStr, "\ntemplate: {\n")
	ExpectWithOffset(2, start).To(BeNumerically(">=", 0), cueStr)
	body := cueStr[start+len("\ntemplate: {\n"):]
	return body[:strings.LastIndex(body, "}")]
}

// evalTemplate fills the policy template with a context and parameter, as the
// controller does, and returns the evaluated template.
func evalTemplate(cueStr, contextJSON, parameterJSON string) cue.Value {
	def := defkitTemplateOnly(cueStr)
	ctx := cuecontext.New()
	v := ctx.CompileString(def + "\ncontext: " + contextJSON + "\nparameter: " + parameterJSON)
	ExpectWithOffset(1, v.Err()).NotTo(HaveOccurred(), def)
	ExpectWithOffset(1, v.Validate(cue.Concrete(true))).To(Succeed(), def)
	return v
}

var _ = Describe("Application-scoped policies", func() {
	sidecar := func() *defkit.PolicyDefinition {
		vela := defkit.VelaCtx()
		return defkit.NewPolicy("sidecar-injector").
			ApplicationScoped().
			Params(defkit.String("image").Default("fluent-bit:3.0")).
			Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Enabled(defkit.Not(defkit.PathExists(`context.appAnnotations["logging.io/opt-out"]`)))
				tpl.Output().
					Components(defkit.AppComponents().AddTrait("sidecar", map[string]defkit.Value{
						"name":  defkit.Lit("logger"),
						"image": defkit.ParamRef("image"),
					})).
					Annotation("logging.io/injected-by", vela.PolicyName()).
					Ctx("logging.sidecar", defkit.ParamRef("image"))
			})
	}

	It("writes scope, global and priority where the spec reads them", func() {
		spec := policySpec(defkit.NewPolicy("p").Global().Priority(100).
			Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Output().Label("compliance.io/managed-by", defkit.Lit("kubevela"))
			}).ToCue())
		Expect(spec.Scope).To(Equal(v1beta1.ApplicationScope))
		Expect(spec.Global).To(BeTrue())
		Expect(spec.Priority).To(Equal(int32(100)))
	})

	It("leaves a policy without them output-scoped", func() {
		spec := policySpec(defkit.NewPolicy("p").
			Template(func(tpl *defkit.PolicyTemplate) { tpl.Set("x", defkit.Lit(1)) }).ToCue())
		Expect(spec.Scope).To(BeEmpty())
		Expect(spec.Global).To(BeFalse())
		Expect(spec.Priority).To(BeZero())
	})

	It("renders a template the controller can evaluate", func() {
		v := evalTemplate(sidecar().ToCue(), `{
			policyName: "logging"
			appAnnotations: {}
			appComponents: [
				{name: "a", type: "webservice", properties: {}},
				{name: "b", type: "webservice", properties: {}, traits: [{type: "scaler", properties: replicas: 2}]},
			]
		}`, `{image: "fluent-bit:3.1"}`)

		// config.enabled is the documented form; admission refuses an expression at the root.
		Expect(v.LookupPath(cue.ParsePath("enabled")).Exists()).To(BeFalse())
		Expect(v.LookupPath(cue.ParsePath("config.enabled")).Bool()).To(BeTrue())
		var out struct {
			Components []struct {
				Name   string `json:"name"`
				Traits []struct {
					Type       string         `json:"type"`
					Properties map[string]any `json:"properties"`
				} `json:"traits"`
			} `json:"components"`
			Annotations map[string]string         `json:"annotations"`
			Ctx         map[string]map[string]any `json:"ctx"`
		}
		Expect(v.LookupPath(cue.ParsePath("output")).Decode(&out)).To(Succeed())
		Expect(out.Components).To(HaveLen(2))
		Expect(out.Components[0].Traits).To(HaveLen(1))
		Expect(out.Components[0].Traits[0].Type).To(Equal("sidecar"))
		Expect(out.Components[0].Traits[0].Properties).To(Equal(map[string]any{"name": "logger", "image": "fluent-bit:3.1"}))
		Expect(out.Components[1].Traits).To(HaveLen(2))
		Expect(out.Components[1].Traits[0].Type).To(Equal("scaler"))
		Expect(out.Annotations).To(Equal(map[string]string{"logging.io/injected-by": "logging"}))
		Expect(out.Ctx).To(Equal(map[string]map[string]any{"logging": {"sidecar": "fluent-bit:3.1"}}))
	})

	It("disables itself when the Application opts out", func() {
		v := evalTemplate(sidecar().ToCue(), `{
			policyName: "logging"
			appAnnotations: "logging.io/opt-out": "true"
			appComponents: []
		}`, `{image: "x"}`)
		Expect(v.LookupPath(cue.ParsePath("config.enabled")).Bool()).To(BeFalse())
	})

	DescribeTable("Register refuses what only an Application-scoped policy may set",
		func(def defkit.Definition, want string) {
			Expect(func() { defkit.Register(def) }).To(PanicWith(ContainSubstring(want)))
		},
		Entry("priority", defkit.NewPolicy("p").Priority(1), "Application-scoped"),
		Entry("output", defkit.NewPolicy("p").Template(func(tpl *defkit.PolicyTemplate) {
			tpl.Output().Label("a", defkit.Lit("b"))
		}), "Application-scoped"),
		Entry("enabled", defkit.NewPolicy("p").Template(func(tpl *defkit.PolicyTemplate) {
			tpl.Enabled(defkit.Eq(defkit.Lit(1), defkit.Lit(1)))
		}), "Application-scoped"),
	)
})
