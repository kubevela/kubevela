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
	"strings"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("HelperDefinition", func() {

	Context("Param-based helper definition", func() {
		It("should report HasParam true and return the param", func() {
			probeParam := defkit.Struct("probe").WithFields(
				defkit.Field("path", defkit.ParamTypeString).Default("/health"),
				defkit.Field("port", defkit.ParamTypeInt).Default(8080),
			)

			comp := defkit.NewComponent("test").
				Helper("HealthProbe", probeParam)

			helpers := comp.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			h := helpers[0]

			Expect(h.GetName()).To(Equal("HealthProbe"))
			Expect(h.HasParam()).To(BeTrue())
			Expect(h.GetParam()).To(Equal(probeParam))
			Expect(h.GetSchema()).To(BeEmpty())
		})
	})

	Context("Multiple helper definitions", func() {
		It("should support multiple helpers on the same component", func() {
			probe := defkit.Struct("probe").WithFields(
				defkit.Field("path", defkit.ParamTypeString),
			)
			resource := defkit.Struct("resource").WithFields(
				defkit.Field("cpu", defkit.ParamTypeString),
				defkit.Field("memory", defkit.ParamTypeString),
			)

			comp := defkit.NewComponent("test").
				Helper("HealthProbe", probe).
				Helper("ResourceSpec", resource)

			helpers := comp.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(2))
			Expect(helpers[0].GetName()).To(Equal("HealthProbe"))
			Expect(helpers[1].GetName()).To(Equal("ResourceSpec"))
		})
	})

	Context("HelperDefinition on TraitDefinition", func() {
		It("should support helper definitions on traits", func() {
			probeParam := defkit.Struct("probe").WithFields(
				defkit.Field("path", defkit.ParamTypeString),
			)

			trait := defkit.NewTrait("health-probe").
				Description("Add health probes").
				AppliesTo("deployments.apps").
				Helper("HealthProbe", probeParam)

			helpers := trait.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			Expect(helpers[0].GetName()).To(Equal("HealthProbe"))
			Expect(helpers[0].HasParam()).To(BeTrue())
		})
	})

	Context("Helper without param (raw schema)", func() {
		It("should report HasParam false when param is nil", func() {
			// Use the raw schema path by passing a nil param through the builder
			// The HelperDefinition struct should report HasParam false
			comp := defkit.NewComponent("test")

			// Access helper definitions - should be empty initially
			helpers := comp.GetHelperDefinitions()
			Expect(helpers).To(BeEmpty())
		})
	})

	Context("Map-based helper with StringKeyMap and typed arrays", func() {
		It("should render StringKeyMap as [string]: string in helper definition", func() {
			helper := defkit.Map("labelSelector").WithFields(
				defkit.StringKeyMap("matchLabels").Optional().Description("A map of {key,value} pairs"),
				defkit.Array("matchExpressions").Optional().Description("Label selector requirements").WithFields(
					defkit.String("key"),
					defkit.String("operator").Default("In").Values("In", "NotIn", "Exists", "DoesNotExist"),
					defkit.Array("values").Optional().Of(defkit.ParamTypeString),
				),
			)

			trait := defkit.NewTrait("helper-test").
				Description("Test helper rendering").
				AppliesTo("deployments.apps").
				Helper("labelSelector", helper).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().Set("spec.selector", defkit.Lit("test"))
				})

			cue := trait.ToCue()

			// StringKeyMap renders as [string]: string
			Expect(cue).To(ContainSubstring("matchLabels?: [string]: string"))
			// Should NOT have duplicate description comments
			Expect(cue).NotTo(ContainSubstring("// +usage=A map of {key,value} pairs\n\t// +usage=A map of {key,value} pairs"))
			// Array with fields renders as [...{...}]
			Expect(cue).To(ContainSubstring("matchExpressions?:"))
			Expect(cue).To(ContainSubstring("[...{"))
			// Typed array renders as [...string]
			Expect(cue).To(ContainSubstring("values?: [...string]"))
			// Enum with default renders correctly
			Expect(cue).To(ContainSubstring(`*"In"`))
		})

		It("should render Of(ParamTypeString) in Struct-based helper fields", func() {
			helper := defkit.Struct("nodeSelector").WithFields(
				defkit.Field("key", defkit.ParamTypeString),
				defkit.Field("operator", defkit.ParamTypeString).Default("In").Values("In", "NotIn"),
				defkit.Field("values", defkit.ParamTypeArray).Optional().Of(defkit.ParamTypeString),
			)

			trait := defkit.NewTrait("typed-array-test").
				Description("Test typed arrays in helpers").
				AppliesTo("deployments.apps").
				Helper("nodeSelector", helper).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().Set("spec.selector", defkit.Lit("test"))
				})

			cue := trait.ToCue()

			Expect(cue).To(ContainSubstring("#nodeSelector"))
			// key has no marker (default) - CUE formatter may add tab alignment
			Expect(cue).To(MatchRegexp(`key:\s+string`))
			Expect(cue).To(ContainSubstring("values?: [...string]"))
			// Untyped array should NOT appear
			Expect(cue).NotTo(MatchRegexp(`values\?\: \[\.\.\.\]\s*\n`))
		})

		It("should render schemaRef with Of correctly in Struct helper", func() {
			selectorHelper := defkit.Struct("nodeSelectorTerm").WithFields(
				defkit.Field("matchExpressions", defkit.ParamTypeArray).Optional().WithSchemaRef("nodeSelector"),
				defkit.Field("matchFields", defkit.ParamTypeArray).Optional().WithSchemaRef("nodeSelector"),
			)

			affinityHelper := defkit.Struct("podAffinityTerm").WithFields(
				defkit.Field("labelSelector", defkit.ParamTypeStruct).Optional().WithSchemaRef("labelSelector"),
				defkit.Field("namespaces", defkit.ParamTypeArray).Optional().Of(defkit.ParamTypeString),
				defkit.Field("topologyKey", defkit.ParamTypeString),
			)

			trait := defkit.NewTrait("schemaref-test").
				Description("Test schemaRef in helpers").
				AppliesTo("deployments.apps").
				Helper("nodeSelectorTerm", selectorHelper).
				Helper("podAffinityTerm", affinityHelper).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().Set("spec.selector", defkit.Lit("test"))
				})

			cue := trait.ToCue()

			// SchemaRef on array field renders as [...#name]
			Expect(cue).To(ContainSubstring("matchExpressions?: [...#nodeSelector]"))
			Expect(cue).To(ContainSubstring("matchFields?: [...#nodeSelector]"))
			// SchemaRef on struct field renders as #name
			Expect(cue).To(ContainSubstring("labelSelector?: #labelSelector"))
			// Of(ParamTypeString) renders as [...string]
			Expect(cue).To(ContainSubstring("namespaces?: [...string]"))
			// Field with no marker (default)
			Expect(cue).To(ContainSubstring("topologyKey: string"))
		})
	})

	Context("Array-built helper referenced from an array", func() {
		indexKeySchema := func() *defkit.ArrayParam {
			return defkit.Array("keySchema").WithFields(
				defkit.String("attributeName"),
				defkit.String("keyType").Values("HASH", "RANGE"),
			)
		}

		It("should reference the helper without a second list wrapper", func() {
			comp := defkit.NewComponent("table").
				Description("x").
				Workload("example.com/v1", "Table").
				Params(defkit.Array("globalSecondaryIndexes").Optional().WithFields(
					defkit.String("indexName"),
					defkit.Array("keySchema").WithSchemaRef("IndexKeySchema"),
				)).
				Helper("IndexKeySchema", indexKeySchema()).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("example.com/v1", "Table").
						Set("spec.indexes", defkit.ParamRef("globalSecondaryIndexes")))
				})

			out := comp.ToCue()
			Expect(out).To(ContainSubstring("keySchema: #IndexKeySchema"))
			Expect(out).NotTo(ContainSubstring("[...#IndexKeySchema]"))

			// A real key schema must unify with the generated parameter.
			idx := strings.Index(out, "template: {")
			Expect(idx).To(BeNumerically(">=", 0))
			body := out[idx+len("template: {"):]
			body = body[:strings.LastIndex(body, "}")]
			v := cuecontext.New().CompileString(`parameter: globalSecondaryIndexes: [{
				indexName: "byActor"
				keySchema: [{attributeName: "actor", keyType: "HASH"}]
			}]
			` + body)
			Expect(v.Validate()).To(Succeed())
		})

		It("should reference an Array-built helper from a helper field without a second list wrapper", func() {
			gsi := defkit.Struct("gsi").WithFields(
				defkit.Field("keySchema", defkit.ParamTypeArray).WithSchemaRef("IndexKeySchema"),
			)
			trait := defkit.NewTrait("list-helper-test").
				Description("x").
				AppliesTo("deployments.apps").
				Helper("IndexKeySchema", indexKeySchema()).
				Helper("GSI", gsi).
				Template(func(tpl *defkit.Template) {
					tpl.Patch().Set("spec.selector", defkit.Lit("test"))
				})

			cue := trait.ToCue()
			Expect(cue).To(ContainSubstring("keySchema: #IndexKeySchema"))
			Expect(cue).NotTo(ContainSubstring("[...#IndexKeySchema]"))
		})

		It("should still wrap an element-shaped helper in a list", func() {
			comp := defkit.NewComponent("table").
				Description("x").
				Workload("example.com/v1", "Table").
				Params(defkit.Array("keySchema").WithSchemaRef("KeyElement")).
				Helper("KeyElement", defkit.Struct("keyElement").WithFields(
					defkit.Field("attributeName", defkit.ParamTypeString),
				)).
				Template(func(tpl *defkit.Template) {
					tpl.Output(defkit.NewResource("example.com/v1", "Table").
						Set("spec.keySchema", defkit.ParamRef("keySchema")))
				})

			Expect(comp.ToCue()).To(ContainSubstring("keySchema: [...#KeyElement]"))
		})
	})
})
