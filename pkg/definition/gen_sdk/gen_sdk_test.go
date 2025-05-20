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
	"context"
	"os"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _outputDir string

var _ = Describe("Test Generating SDK", func() {
	var err error
	outputDir := filepath.Join("testdata", "output")
	_outputDir = outputDir
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
		err = meta.Run(context.Background())
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

})

var _ = AfterSuite(func() {
	By("Cleaning up generated files")
	_ = os.RemoveAll(_outputDir)
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
							Type: typeNumber,
						},
					},
					{
						Value: &openapi3.Schema{
							Type: typeString,
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
				Type:  typeString,
				Title: "image",
				OneOf: openapi3.SchemaRefs{
					{
						Value: &openapi3.Schema{
							Type: typeString,
							Enum: []interface{}{"go", "java", "python", "node", "ruby"},
						},
					},
					{
						Value: &openapi3.Schema{
							Type: typeString,
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
				Type:  typeString,
				Title: "image",
				OneOf: openapi3.SchemaRefs{
					{
						Value: &openapi3.Schema{
							Enum: []interface{}{"go", "java", "python", "node", "ruby"},
						},
					},
					{
						Value: &openapi3.Schema{
							Type: typeString,
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
				langArgs: []string{"GoProxy=value1", "MainModuleVersion=value2"},
			},
			want:    map[string]string{"GoProxy": "value1", "MainModuleVersion": "value2"},
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
			Expect(err).Should(BeNil())
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
	typeInvalid := &openapi3.Types{"anyschema"}

	BeforeEach(func() {
		schema = &openapi3.Schema{Type: typeString}
	})

	var testCases = []struct {
		name        string
		cueType     CUEType
		expectedFit bool
		schemaType  *openapi3.Types
	}{
		{"string can be oas string", CUEType("string"), true, typeString},
		{"string not oas integer", CUEType("string"), false, typeInteger},
		{"integer can be oas integer", CUEType("integer"), true, typeInteger},
		{"integer can be oas number", CUEType("integer"), true, typeNumber},
		{"number can be oas number", CUEType("number"), true, typeNumber},
		{"number not oas integer", CUEType("number"), false, typeInteger},
		{"boolean can be oas boolean", CUEType("boolean"), true, typeBoolean},
		{"array can be oas array", CUEType("array"), true, typeArray},
		{"invalid type and any schema", CUEType(""), false, typeInvalid},
	}

	It("should return whether the CUEType fits the schema type or not", func() {
		for _, tc := range testCases {
			schema.Type = tc.schemaType
			result := tc.cueType.fit(schema)
			Expect(result).To(Equal(tc.expectedFit), tc.name)
		}
	})
})
