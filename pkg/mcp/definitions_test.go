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

package mcp

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

// definitionFixtures adds a platform-wide default and a Terraform-schematic
// component to the platform fixtures.
func definitionFixtures() []client.Object {
	return append(platformFixtures(),
		&v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "postgres",
				Namespace:   types.DefaultKubeVelaNS,
				Labels:      map[string]string{providesLabel: "postgres"},
				Annotations: map[string]string{defaultForAnnotation: "*"},
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "output: kind: \"ConfigMap\"\nparameter: size: *\"1Gi\" | string\n"}},
			},
		},
		&v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "rds", Namespace: types.DefaultKubeVelaNS},
			Spec: v1beta1.ComponentDefinitionSpec{
				Schematic: &oamcommon.Schematic{Terraform: &oamcommon.Terraform{Configuration: "variable \"instance_class\" {}\n"}},
			},
		},
	)
}

var _ = Describe("definition description", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	It("marks the platform default in describe_definition and explain_definition, as list_definitions does", func() {
		h = startHarness(Options{}, definitionFixtures()...)
		var list ListDefinitionsOutput
		h.call("list_definitions", map[string]any{"provides": "postgres", "namespace": "team-a"}, &list)
		Expect(list.Definitions).To(ContainElement(HaveField("Default", BeTrue())))

		var desc DescribeDefinitionOutput
		res := h.call("describe_definition", map[string]any{"name": "postgres", "namespace": "team-a"}, &desc)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(desc.Default).To(BeTrue())

		var expl ExplainDefinitionOutput
		res = h.call("explain_definition", map[string]any{"name": "postgres", "namespace": "team-a"}, &expl)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(expl.Default).To(BeTrue())
	})

	It("says the parameter schema of a Terraform schematic is unavailable instead of reporting no parameters", func() {
		h = startHarness(Options{}, definitionFixtures()...)
		var desc DescribeDefinitionOutput
		res := h.call("describe_definition", map[string]any{"name": "rds", "type": "component"}, &desc)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(desc.Parameters).To(BeEmpty())
		Expect(desc.Example).To(BeNil(), "an example built from no schema would read as taking no parameters")
		Expect(desc.Meta.Warnings).To(ContainElement(ContainSubstring("parameter schema for a Terraform schematic is not available from this server")))

		var expl ExplainDefinitionOutput
		res = h.call("explain_definition", map[string]any{"name": "rds", "type": "component"}, &expl)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(expl.Meta.Warnings).To(ContainElement(ContainSubstring("parameter schema for a Terraform schematic is not available from this server")))
	})
})

// CUE's OpenAPI encoder alone fails a list with a minimum size with "invalid
// value [] (does not satisfy list.MinItems(1))" and silently drops a maximum
// size; the controller's generator reads both.
var _ = Describe("parameter schema of size-bounded lists", func() {
	const template = `import (
	"list"
	"strings"
)

output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	data: name: parameter.keySchema[0].attributeName
}
#KeySchemaElement: {
	attributeName!: string & strings.MinRunes(1)
	keyType!:       "HASH" | "RANGE"
}
parameter: {
	attributeDefinitions!: list.MinItems(1) & [...{
		attributeName!: string & strings.MinRunes(1) & strings.MaxRunes(255)
		attributeType!: "S" | "N" | "B"
	}]
	keySchema!: list.MinItems(1) & list.MaxItems(2) & [...#KeySchemaElement]
	nonKey?:    list.MinItems(1) & [...string]
	tags?:      list.MaxItems(50) & [...string]
}
`

	It("keeps minItems and maxItems instead of failing", func() {
		params, required, sch, err := parameterSchema(context.Background(), template)
		Expect(err).NotTo(HaveOccurred())
		Expect(required).To(ConsistOf("attributeDefinitions", "keySchema"))

		props := params["properties"].(map[string]any)
		Expect(props["attributeDefinitions"]).To(And(HaveKeyWithValue("type", "array"), HaveKeyWithValue("minItems", BeEquivalentTo(1))))
		Expect(props["keySchema"]).To(And(HaveKeyWithValue("type", "array"), HaveKeyWithValue("minItems", BeEquivalentTo(1)), HaveKeyWithValue("maxItems", BeEquivalentTo(2))))
		Expect(props["nonKey"]).To(And(HaveKeyWithValue("type", "array"), HaveKeyWithValue("minItems", BeEquivalentTo(1))))
		Expect(props["tags"]).To(And(HaveKeyWithValue("type", "array"), HaveKeyWithValue("maxItems", BeEquivalentTo(50))))
		Expect(sch.Properties["keySchema"].Value.Items.Value.Properties).To(HaveKey("attributeName"))
	})
})
