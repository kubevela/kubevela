/*
Copyright 2021 The KubeVela Authors.

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

package gen_sdk

import (
	"os"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test Generating SDK", func() {
	var err error
	outputDir := filepath.Join("testdata", "output")
	meta := GenMeta{
		Output:  outputDir,
		Lang:    "go",
		Package: "github.com/kubevela/test-gen-sdk",
		File:    []string{filepath.Join("testdata", "cron-task.cue")},
		InitSDK: true,
	}
	checkDirNotEmpty := func(dir string) {
		_, err = os.Stat(dir)
		Expect(err).Should(BeNil())
	}
	genWithMeta := func(meta GenMeta) {
		err = meta.Init(common.Args{})
		Expect(err).Should(BeNil())
		err = meta.CreateScaffold()
		Expect(err).Should(BeNil())
		err = meta.PrepareGeneratorAndTemplate()
		Expect(err).Should(BeNil())
		err = meta.Run()
		Expect(err).Should(BeNil())
	}
	It("Test generating SDK and init the scaffold", func() {
		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis"))
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "component", "cron-task"))
	})

	It("Test generating SDK, append apis", func() {
		meta.InitSDK = false
		meta.File = append(meta.File, "testdata/shared-resource.cue")

		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "policy", "shared-resource"))
	})

	It("Test free form parameter {...}", func() {
		meta.InitSDK = false
		meta.File = []string{"testdata/json-merge-patch.cue"}
		meta.Verbose = true

		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "trait", "json-merge-patch"))
	})

	It("Test workflow step", func() {
		meta.InitSDK = false
		meta.File = []string{"testdata/deploy.cue"}
		meta.Verbose = true

		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "deploy"))
	})

	It("Test step-group", func() {
		meta.InitSDK = false
		meta.File = []string{"testdata/step-group.cue"}
		meta.Verbose = true

		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "step-group"))
		By("check if AddSubStep is generated")
		content, err := os.ReadFile(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "step-group", "step_group.go"))
		Expect(err).Should(BeNil())
		Expect(string(content)).Should(ContainSubstring("AddSubStep"))
	})

	It("Test oneOf", func() {
		meta.InitSDK = false
		meta.File = []string{"testdata/one_of.cue"}
		meta.Verbose = true

		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "one_of"))
		By("check if ")
	})

	It("Test known issue: apply-terraform-provider", func() {
		meta.InitSDK = false
		meta.Verbose = true
		meta.File = []string{"testdata/apply-terraform-provider.cue"}
		genWithMeta(meta)
	})

	AfterSuite(func() {
		By("Cleaning up generated files")
		_ = os.RemoveAll(outputDir)
	})

})

var _ = Describe("FixSchemaWithOneAnyAllOf", func() {
	var (
		schema *openapi3.SchemaRef
	)

	It("should set default value to right sub-schema", func() {
		By(`cpu?: *1 | number | string`)
		schema = &openapi3.SchemaRef{
			Ref: "",
			Value: &openapi3.Schema{
				Default: 1,
				OneOf: openapi3.SchemaRefs{
					{
						Value: &openapi3.Schema{
							Type: "number",
						},
					},
					{
						Value: &openapi3.Schema{
							Type: "string",
						},
					},
				},
			},
		}
		fixSchemaWithOneOf(schema)

		Expect(schema.Value.OneOf[0].Value.Default).To(Equal(1))
		Expect(schema.Value.OneOf[1].Value.Default).To(BeNil())
		Expect(schema.Value.Default).To(BeNil())
	})

	It("should remove duplicated type in oneOf", func() {
		By(`language: "go" | "java" | "python" | "node" | "ruby" | string`)
		By(`image: language | string`)
		schema = &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:  "string",
				Title: "image",
				OneOf: openapi3.SchemaRefs{
					{
						Value: &openapi3.Schema{
							Type: "string",
							Enum: []interface{}{"go", "java", "python", "node", "ruby"},
						},
					},
					{
						Value: &openapi3.Schema{
							Type: "string",
						},
					},
				},
			},
		}
		fixSchemaWithOneOf(schema)

		Expect(schema.Value.OneOf).To(HaveLen(1))
		Expect(schema.Value.OneOf[0].Value.Type).To(Equal("string"))
		Expect(schema.Value.OneOf[0].Value.Enum).To(Equal([]interface{}{"go", "java", "python", "node", "ruby"}))
	})

	It("should both move type and remove duplicated type in oneOf", func() {
		schema = &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Type:  "string",
				Title: "image",
				OneOf: openapi3.SchemaRefs{
					{
						Value: &openapi3.Schema{
							Enum: []interface{}{"go", "java", "python", "node", "ruby"},
						},
					},
					{
						Value: &openapi3.Schema{
							Type: "string",
						},
					},
				},
			},
		}

		fixSchemaWithOneOf(schema)
		Expect(schema.Value.OneOf).To(HaveLen(1))
		Expect(schema.Value.OneOf[0].Value.Type).To(Equal("string"))
		Expect(schema.Value.OneOf[0].Value.Enum).To(Equal([]interface{}{"go", "java", "python", "node", "ruby"}))
	})
})
