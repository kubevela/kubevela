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
	lang := "go"
	meta := GenMeta{
		Output:       outputDir,
		Lang:         lang,
		Package:      "github.com/kubevela-contrib/kubevela-go-sdk",
		APIDirectory: defaultAPIDir[lang],
		Verbose:      true,
	}
	var langArgs []string

	BeforeEach(func() {
		meta.InitSDK = false
		meta.File = []string{filepath.Join("testdata", "cron-task.cue")}
		meta.cuePaths = []string{}
	})

	checkDirNotEmpty := func(dir string) {
		_, err = os.Stat(dir)
		Expect(err).Should(BeNil())
	}
	genWithMeta := func() {
		err = meta.Init(common.Args{}, langArgs)
		Expect(err).Should(BeNil())
		err = meta.CreateScaffold()
		Expect(err).Should(BeNil())
		err = meta.PrepareGeneratorAndTemplate()
		Expect(err).Should(BeNil())
		err = meta.Run()
		Expect(err).Should(BeNil())
	}
	It("Test generating SDK and init the scaffold", func() {
		meta.InitSDK = true
		genWithMeta()
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis"))
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "component", "cron-task"))
	})

	It("Test generating SDK, append apis", func() {
		meta.File = append(meta.File, "testdata/shared-resource.cue")

		genWithMeta()
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "policy", "shared-resource"))
	})

	It("Test free form parameter {...}", func() {
		meta.File = []string{"testdata/json-merge-patch.cue"}
		meta.Verbose = true

		genWithMeta()
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "trait", "json-merge-patch"))
	})

	It("Test workflow step", func() {
		meta.File = []string{"testdata/deploy.cue"}
		meta.Verbose = true

		genWithMeta()
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "deploy"))
	})

	It("Test step-group", func() {
		meta.File = []string{"testdata/step-group.cue"}
		meta.Verbose = true

		genWithMeta()
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "step-group"))
		By("check if AddSubStep is generated")
		content, err := os.ReadFile(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "step-group", "step_group.go"))
		Expect(err).Should(BeNil())
		Expect(string(content)).Should(ContainSubstring("AddSubStep"))
	})

	It("Test oneOf", func() {
		meta.File = []string{"testdata/one_of.cue"}
		meta.Verbose = true

		genWithMeta()
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "workflow-step", "one_of"))
	})

	It("Test known issue: apply-terraform-provider", func() {
		meta.Verbose = true
		meta.File = []string{"testdata/apply-terraform-provider.cue"}
		genWithMeta()
	})

	It("Test generate sub-module", func() {
		meta.APIDirectory = "pkg/apis/addons/test_addon"
		langArgs = []string{
			string(mainModuleVersionKey) + "=" + mainModuleVersion.Default,
		}
		meta.IsSubModule = true
		genWithMeta()
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "addons", "test_addon", "component", "cron-task"))
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

var _ = Describe("TestNewLanguageArgs", func() {
	type args struct {
		lang     string
		langArgs []string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "should create a languageArgs struct with the correct values",
			args: args{
				lang:     "go",
				langArgs: []string{"flag1=value1", "flag2=value2"},
			},
			want:    map[string]string{"flag1": "value1", "flag2": "value2"},
			wantErr: false,
		},
		{
			name: "should not set a value for an unknown flag",
			args: args{
				lang:     "go",
				langArgs: []string{"unknownFlag=value"},
			},
			want:    map[string]string{},
			wantErr: true,
		},
		{
			name: "should warn if an argument is not in the key=value format",
			args: args{
				lang:     "go",
				langArgs: []string{"invalidArgument"},
			},
			want:    map[string]string{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		It(tt.name, func() {
			got, err := NewLanguageArgs(tt.args.lang, tt.args.langArgs)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			for k, v := range tt.want {
				Expect(got.Get(langArgKey(k))).To(Equal(v))
			}
		})
	}

})

var _ = Describe("getValueType", func() {

	type valueTypeTest struct {
		input    interface{}
		expected CUEType
	}

	tests := []valueTypeTest{
		{nil, ""},
		{"hello", "string"},
		{42, "integer"},
		{float32(3.14), "number"},
		{3.14159265358979323846, "number"},
		{true, "boolean"},
		{map[string]interface{}{"key": "value"}, "object"},
		{[]interface{}{1, 2, 3}, "array"},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		It("should return the correct CUEType for the input", func() {
			Expect(getValueType(tt.input)).To(Equal(tt.expected))
		})
	}
})

var _ = Describe("type fit", func() {

	var schema *openapi3.Schema

	BeforeEach(func() {
		schema = &openapi3.Schema{Type: "string"}
	})

	var testCases = []struct {
		name        string
		cueType     CUEType
		expectedFit bool
		schemaType  string
	}{
		{"string can be oas string", CUEType("string"), true, "string"},
		{"string not oas integer", CUEType("string"), false, "integer"},
		{"integer can be oas integer", CUEType("integer"), true, "integer"},
		{"integer can be oas number", CUEType("integer"), true, "number"},
		{"number can be oas number", CUEType("number"), true, "number"},
		{"number not oas integer", CUEType("number"), false, "integer"},
		{"boolean can be oas boolean", CUEType("boolean"), true, "boolean"},
		{"array can be oas array", CUEType("array"), true, "array"},
		{"invalid type and any schema", CUEType(""), false, "anyschema"},
	}

	It("should return whether the CUEType fits the schema type or not", func() {
		for _, tc := range testCases {
			schema.Type = tc.schemaType
			result := tc.cueType.fit(schema)
			Expect(result).To(Equal(tc.expectedFit), tc.name)
		}
	})
})
