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
	"bytes"
	"encoding/json"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

// specAs parses generated CUE the way apply-module does and decodes the spec
// strictly into out.
func specAs(cueStr string, out any) {
	bs, err := json.Marshal(fromCUE(cueStr))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	dec := json.NewDecoder(bytes.NewReader(bs))
	dec.DisallowUnknownFields()
	ExpectWithOffset(1, dec.Decode(out)).To(Succeed())
}

var _ = Describe("Definition inheritance", func() {
	vela := defkit.VelaCtx()

	baseWeb := func() *defkit.ComponentDefinition {
		return defkit.NewComponent("base-web").
			Workload("apps/v1", "Deployment").
			Params(
				defkit.String("image"),
				defkit.Int("replicas").Default(1),
			).
			Template(func(tpl *defkit.Template) {
				tpl.Output(defkit.NewResource("apps/v1", "Deployment").
					Set("metadata.name", vela.Name()).
					Set("spec.replicas", defkit.ParamRef("replicas")).
					Set("spec.template.spec.containers[0].name", vela.Name()).
					Set("spec.template.spec.containers[0].image", defkit.ParamRef("image")))
			})
	}

	tenantWeb := func(parent *defkit.ComponentDefinition) *defkit.ComponentDefinition {
		return defkit.NewComponent("tenant-web").
			Extends(parent).
			Params(defkit.String("image"), defkit.String("tenant")).
			Template(func(tpl *defkit.Template) {
				tpl.Super().Set("image", defkit.ParamRef("image"))
				tpl.Output(defkit.NewResource("apps/v1", "Deployment").
					Set("metadata.labels[tenant.oam.dev/name]", defkit.ParamRef("tenant")))
				tpl.Outputs("tenantConfig", defkit.NewResource("v1", "ConfigMap").
					Set("metadata.name", defkit.Interpolation(vela.Name(), defkit.Lit("-tenant"))).
					Set("data.tenant", defkit.ParamRef("tenant")).
					Set("data.parent", defkit.SuperOutput("metadata.name")))
			})
	}

	// webserviceRef references the built-in webservice, with the CUE it ships.
	webserviceRef := func() *defkit.ComponentDefinition {
		src, err := os.ReadFile("../../../vela-templates/definitions/internal/component/webservice.cue")
		Expect(err).NotTo(HaveOccurred())
		return defkit.ComponentRef("webservice").WithCUE(string(src))
	}

	tenantWebservice := func(parent *defkit.ComponentDefinition) *defkit.ComponentDefinition {
		return defkit.NewComponent("tenant-webservice").
			Extends(parent).
			Params(defkit.String("image"), defkit.String("tenant")).
			Template(func(tpl *defkit.Template) {
				tpl.Super().Set("image", defkit.ParamRef("image"))
				tpl.Output(defkit.NewResource("apps/v1", "Deployment").
					Set("metadata.labels[tenant.oam.dev/name]", defkit.ParamRef("tenant")))
			})
	}

	Context("the header", func() {
		It("writes extends and abstract where the spec reads them", func() {
			var spec v1beta1.ComponentDefinitionSpec
			specAs(tenantWeb(baseWeb().Abstract()).ToCue(), &spec)
			Expect(spec.Extends).To(Equal("base-web"))
			Expect(spec.Abstract).To(BeFalse())

			var parent v1beta1.ComponentDefinitionSpec
			specAs(baseWeb().Abstract().ToCue(), &parent)
			Expect(parent.Abstract).To(BeTrue())
		})

		It("pins a revision by name", func() {
			var spec v1beta1.ComponentDefinitionSpec
			specAs(defkit.NewComponent("c").Extends(defkit.ComponentRef("webservice@v3")).
				Template(func(tpl *defkit.Template) { tpl.Super() }).ToCue(), &spec)
			Expect(spec.Extends).To(Equal("webservice@v3"))
		})

		It("leaves the workload to the parent unless one is set", func() {
			var spec v1beta1.ComponentDefinitionSpec
			specAs(tenantWeb(baseWeb()).ToCue(), &spec)
			Expect(spec.Workload.Type).To(BeEmpty())
			Expect(spec.Workload.Definition.Kind).To(BeEmpty())
		})

		It("writes a referenced trait's name and pin", func() {
			var spec v1beta1.TraitDefinitionSpec
			specAs(defkit.NewTrait("t").Extends(defkit.TraitRef("labels@v2")).
				Template(func(tpl *defkit.Template) { tpl.Super() }).ToCue(), &spec)
			Expect(spec.Extends).To(Equal("labels@v2"))
		})

		It("writes a trait's extends and abstract", func() {
			parent := defkit.NewTrait("base-labels").Abstract().
				Params(defkit.String("a")).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().Set("metadata.labels.a", defkit.ParamRef("a"))
				})
			child := defkit.NewTrait("team-labels").Extends(parent).
				Params(defkit.String("team")).
				Template(func(tpl *defkit.Template) {
					tpl.Super().Set("a", defkit.ParamRef("team"))
					tpl.Patch().Set("metadata.labels.team", defkit.ParamRef("team"))
				})

			var spec v1beta1.TraitDefinitionSpec
			specAs(child.ToCue(), &spec)
			Expect(spec.Extends).To(Equal("base-labels"))
			specAs(parent.ToCue(), &spec)
			Expect(spec.Abstract).To(BeTrue())
		})
	})

	Context("the template", func() {
		It("writes $super and $inherit", func() {
			c := defkit.NewComponent("c").Extends(defkit.ComponentRef("p")).
				Template(func(tpl *defkit.Template) {
					tpl.Super().Set("image", defkit.ParamRef("image"))
					tpl.InheritField("outputs", false)
				})
			Expect(c.ToCue()).To(ContainSubstring("$super: properties: {"))
			Expect(c.ToCue()).To(ContainSubstring("image: parameter.image"))
			Expect(c.ToCue()).To(ContainSubstring("$inherit: outputs: false"))

			c = defkit.NewComponent("c").Extends(defkit.ComponentRef("p")).
				Template(func(tpl *defkit.Template) {
					tpl.Super()
					tpl.Inherit(false)
				})
			Expect(c.ToCue()).To(ContainSubstring("$super: properties: {}"))
			Expect(c.ToCue()).To(ContainSubstring("$inherit: false"))
		})
	})

	Context("EvaluateCUE", func() {
		It("renders the child on top of its parent, as the controller does", func() {
			out, err := tenantWeb(baseWeb()).EvaluateCUE(defkit.TestContext().
				WithName("shop").
				WithParam("image", "nginx:1.27").
				WithParam("tenant", "acme"))
			Expect(err).NotTo(HaveOccurred())

			Expect(out.Primary).To(HaveKeyWithValue("kind", "Deployment"))
			Expect(out.Primary["metadata"]).To(Equal(map[string]any{
				"name":   "shop",
				"labels": map[string]any{"tenant.oam.dev/name": "acme"},
			}))
			Expect(out.Primary["spec"]).To(HaveKeyWithValue("replicas", BeNumerically("==", 1)))
			Expect(out.Auxiliary).To(HaveKey("tenantConfig"))
			Expect(out.Auxiliary["tenantConfig"]["data"]).To(Equal(map[string]any{
				"tenant": "acme",
				"parent": "shop",
			}))
		})

		It("asks for the CUE of a parent it only references", func() {
			_, err := defkit.NewComponent("c").Extends(defkit.ComponentRef("webservice")).
				Template(func(tpl *defkit.Template) { tpl.Super() }).
				EvaluateCUE(defkit.TestContext())
			Expect(err).To(MatchError(ContainSubstring(`ComponentRef("webservice").WithCUE`)))
		})

		It("renders on a referenced parent given its CUE", func() {
			out, err := tenantWebservice(webserviceRef()).EvaluateCUE(defkit.TestContext().
				WithName("shop").
				WithParam("image", "nginx:1.27").
				WithParam("tenant", "acme"))
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Primary["metadata"]).To(Equal(map[string]any{
				"labels": map[string]any{"tenant.oam.dev/name": "acme"},
			}))
			Expect(out.Primary).To(HaveKeyWithValue("spec", HaveKeyWithValue("template",
				HaveKeyWithValue("spec", HaveKeyWithValue("containers", ContainElement(SatisfyAll(
					HaveKeyWithValue("name", "shop"),
					HaveKeyWithValue("image", "nginx:1.27"),
				)))))))
		})

		It("is what Render points a child at", func() {
			Expect(func() { tenantWeb(baseWeb()).Render(defkit.TestContext()) }).
				To(PanicWith(ContainSubstring("EvaluateCUE")))
		})
	})

	Context("Register", func() {
		It("accepts a child that calls its parent with what it requires", func() {
			defer defkit.Clear()
			Expect(func() { defkit.Register(tenantWeb(baseWeb())) }).NotTo(Panic())
		})

		It("refuses a child that never calls its parent", func() {
			child := defkit.NewComponent("c").Extends(baseWeb()).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("apps/v1", "Deployment"))
				})
			Expect(func() { defkit.Register(child) }).To(PanicWith(ContainSubstring("never calls it")))
		})

		It("checks a call to a referenced parent given its CUE", func() {
			child := defkit.NewComponent("c").Extends(webserviceRef()).
				Template(func(tpl *defkit.Template) { tpl.Super() })
			Expect(func() { defkit.Register(child) }).To(PanicWith(ContainSubstring(`"image"`)))
		})

		It("leaves a call to a bare reference to admission", func() {
			defer defkit.Clear()
			child := defkit.NewComponent("c").Extends(defkit.ComponentRef("webservice")).
				Template(func(tpl *defkit.Template) { tpl.Super() })
			Expect(func() { defkit.Register(child) }).NotTo(Panic())
		})

		It("refuses to register a reference", func() {
			Expect(func() { defkit.Register(webserviceRef()) }).To(PanicWith(ContainSubstring("installed elsewhere")))
			Expect(func() { defkit.Register(defkit.TraitRef("labels")) }).To(PanicWith(ContainSubstring("installed elsewhere")))
		})

		It("refuses a call missing a parameter the parent requires", func() {
			child := defkit.NewComponent("c").Extends(baseWeb()).
				Template(func(tpl *defkit.Template) { tpl.Super() })
			Expect(func() { defkit.Register(child) }).To(PanicWith(ContainSubstring(`"image"`)))
		})
	})
})
