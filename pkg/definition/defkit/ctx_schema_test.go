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

package defkit_test

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
	"github.com/oam-dev/kubevela/pkg/policy/ctxschema"
)

var _ = Describe("Policy context schemas", func() {
	identity := func(closed bool) *defkit.PolicyDefinition {
		fields := []defkit.Param{
			defkit.String("team").Pattern("^[a-z]+$"),
			defkit.String("costCentre").Required(),
			defkit.String("meshDomain").Optional(),
			defkit.Int("replicas").Default(2),
			defkit.Struct("org").WithFields(
				defkit.Field("unit", defkit.ParamTypeString),
				defkit.Field("zone", defkit.ParamTypeString).Optional(),
			),
		}
		p := defkit.NewPolicy("identity").ApplicationScoped().Global()
		if closed {
			p = p.ClosedCtxSchema(fields...)
		} else {
			p = p.CtxSchema(fields...)
		}
		return p.Template(func(tpl *defkit.PolicyTemplate) {
			tpl.Output().
				Ctx("team", defkit.Lit("payments")).
				Ctx("costCentre", defkit.Lit("cc1")).
				Ctx("org.unit", defkit.Lit("u"))
		})
	}

	// schemaOf reads the generated template as admission does.
	schemaOf := func(p *defkit.PolicyDefinition) (ctxschema.Info, cue.Value) {
		info, err := ctxschema.Read(defkitTemplateOnly(p.ToCue()))
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, info.Schema).NotTo(BeEmpty(), p.ToCue())
		schema, err := ctxschema.Compile(cuecontext.New(), info.Schema)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), info.Schema)
		return info, schema
	}
	accepts := func(schema cue.Value, value string) bool {
		return schema.Unify(schema.Context().CompileString(value)).Validate() == nil
	}

	It("writes a schema admission reads on its own", func() {
		info, schema := schemaOf(identity(false))
		Expect(info.SetsCtx).To(BeTrue())
		Expect(accepts(schema, `{team: "payments"}`)).To(BeTrue())
		Expect(accepts(schema, `{team: "Payments"}`)).To(BeFalse(), "the pattern is kept")
		Expect(accepts(schema, `{replicas: "two"}`)).To(BeFalse(), "the type is kept")
		Expect(accepts(schema, `{org: unit: 1}`)).To(BeFalse(), "a nested field is kept")
		Expect(accepts(schema, `{region: "eu"}`)).To(BeTrue(), "CtxSchema is open")
	})

	It("closes the schema when asked", func() {
		_, schema := schemaOf(identity(true))
		Expect(accepts(schema, `{team: "payments"}`)).To(BeTrue())
		Expect(accepts(schema, `{region: "eu"}`)).To(BeFalse())
	})

	It("marks keys required and optional as the parameters are", func() {
		info, _ := schemaOf(identity(true))
		v := ctxschema.Build([]ctxschema.Policy{{Name: "identity", Template: defkitTemplateOnly(identity(true).ToCue())}})
		Expect(v.Complete).To(BeTrue())
		Expect(v.Warnings).To(BeEmpty())
		Expect(info.Conditional).To(BeFalse())
		for key, optional := range map[string]bool{"team": false, "costCentre": false, "meshDomain": true, "replicas": false, "org": false} {
			_, got := v.Optional(key)
			Expect(got).To(Equal(optional), key)
		}
		_, zoneOptional := v.PathOptional([]string{"org", "zone"})
		Expect(zoneOptional).To(BeTrue())
	})

	// Not evalTemplate: a schema is a type, so the template as a whole is not
	// concrete, and the controller does not ask it to be.
	It("renders output that its own schema accepts", func() {
		v := cuecontext.New().CompileString(defkitTemplateOnly(identity(true).ToCue()) + "\ncontext: {}\nparameter: {}")
		Expect(v.Err()).NotTo(HaveOccurred())
		out := v.LookupPath(cue.ParsePath("schema.ctx")).Unify(v.LookupPath(cue.ParsePath("output.ctx")))
		Expect(out.Validate(cue.Concrete(true))).To(Succeed())
	})

	It("is optional wholesale when the policy is conditional", func() {
		p := defkit.NewPolicy("sometimes").ApplicationScoped().
			ClosedCtxSchema(defkit.String("tier")).
			Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Enabled(defkit.PathExists(`context.appAnnotations["tier"]`))
				tpl.Output().Ctx("tier", defkit.Lit("gold"))
			})
		v := ctxschema.Build([]ctxschema.Policy{{Name: "sometimes", Template: defkitTemplateOnly(p.ToCue())}})
		_, optional := v.Optional("tier")
		Expect(optional).To(BeTrue())
	})

	It("adds fields from every call", func() {
		p := defkit.NewPolicy("p").ApplicationScoped().
			CtxSchema(defkit.String("a")).
			CtxSchema(defkit.String("b"))
		_, schema := schemaOf(p)
		Expect(accepts(schema, `{a: 1}`)).To(BeFalse())
		Expect(accepts(schema, `{b: 1}`)).To(BeFalse())
	})

	It("writes no schema when none is declared", func() {
		p := defkit.NewPolicy("p").ApplicationScoped().Template(func(tpl *defkit.PolicyTemplate) {
			tpl.Output().Ctx("team", defkit.Lit("x"))
		})
		info, err := ctxschema.Read(defkitTemplateOnly(p.ToCue()))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Schema).To(BeEmpty())
	})

	DescribeTable("Register refuses a schema that cannot hold",
		func(def defkit.Definition, want string) {
			Expect(func() { defkit.Register(def) }).To(PanicWith(ContainSubstring(want)))
		},
		Entry("on a policy that is not Application-scoped",
			defkit.NewPolicy("p").CtxSchema(defkit.String("team")), "Application-scoped"),
		Entry("an output key a closed schema does not declare",
			defkit.NewPolicy("p").ApplicationScoped().ClosedCtxSchema(defkit.String("team")).Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Output().Ctx("team", defkit.Lit("a")).Ctx("region.name", defkit.Lit("eu"))
			}), `Ctx("region.name")`),
	)

	DescribeTable("Register accepts",
		func(def defkit.Definition) {
			Expect(func() { defkit.Register(def) }).NotTo(Panic())
			defkit.Clear()
		},
		Entry("a nested output key under a declared one",
			defkit.NewPolicy("p").ApplicationScoped().ClosedCtxSchema(defkit.Struct("org").WithFields(defkit.Field("unit", defkit.ParamTypeString))).Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Output().Ctx("org.unit", defkit.Lit("u"))
			})),
		Entry("other output beside a closed schema",
			defkit.NewPolicy("p").ApplicationScoped().ClosedCtxSchema(defkit.String("team")).Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Output().Label("team", defkit.Lit("a")).Annotation("note", defkit.Lit("b")).Ctx("team", defkit.Lit("a"))
			})),
		Entry("a map key under a declared map",
			defkit.NewPolicy("p").ApplicationScoped().ClosedCtxSchema(defkit.Map("tags")).Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Output().Ctx("tags[env]", defkit.Lit("prod"))
			})),
		Entry("an undeclared output key under an open schema",
			defkit.NewPolicy("p").ApplicationScoped().CtxSchema(defkit.String("team")).Template(func(tpl *defkit.PolicyTemplate) {
				tpl.Output().Ctx("region", defkit.Lit("eu"))
			})),
	)

	It("writes a closed schema with no fields as a closed empty ctx", func() {
		p := defkit.NewPolicy("nothing").ApplicationScoped().ClosedCtxSchema().Template(func(tpl *defkit.PolicyTemplate) {
			tpl.Output().Label("a", defkit.Lit("b"))
		})
		Expect(p.ToCue()).To(ContainSubstring("schema: ctx: close({\n"))
		info, err := ctxschema.Read(defkitTemplateOnly(p.ToCue()))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Schema).NotTo(BeEmpty())
	})
})
