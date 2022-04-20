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

package definition

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"
	assert2 "gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestDefaultFieldNamer(t *testing.T) {
	for name, expected := range map[string]string{
		"id":         "ID",
		"foo":        "Foo",
		"foo_bar":    "FooBar",
		"fooBar":     "FooBar",
		"FOO_BAR":    "FooBar",
		"FOO_BAR_ID": "FooBarID",
		"123":        "_123",
		"A|B":        "A_B",
	} {
		assert.Equal(t, expected, dm.FieldName(name))
	}
}

func TestTrimIncompleteKind(t *testing.T) {
	incompleteKinds := []struct {
		kind     string
		expected string
		err      bool
	}{
		{
			kind:     "string",
			expected: "string",
			err:      false,
		},
		{
			kind:     "(null|string)",
			expected: "string",
			err:      false,
		},
		{
			kind: "(null|string|int)",
			err:  true,
		},
	}
	for _, k := range incompleteKinds {
		actual, err := trimIncompleteKind(k.kind)
		if k.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, k.expected, actual)
		}
	}
}

func TestGeneratorParameterStructs(t *testing.T) {
	testCases := []struct {
		name     string
		cue      string
		expected []StructParameter
		err      bool
	}{
		{
			name:     "go struct",
			cue:      defWithStruct,
			err:      false,
			expected: structsWithStruct,
		},
		{
			name:     "go list",
			cue:      defWithList,
			err:      false,
			expected: structsWithList,
		},
		{
			name:     "go struct list",
			cue:      defWithStructList,
			err:      false,
			expected: structsWithStructList,
		},
		{
			name:     "go map",
			cue:      defWithMap,
			err:      false,
			expected: structsWithMap,
		},
		{
			name:     "map element not defined",
			cue:      defWithEmptyMap,
			err:      false,
			expected: structWithInterface,
		},
	}
	for _, tc := range testCases {
		value, err := common.GetCUEParameterValue(tc.cue, nil)
		assert.NoError(t, err)
		actual, err := GeneratorParameterStructs(value)
		if tc.err {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert2.DeepEqual(t, tc.expected, actual)
		}
	}
}

func TestGenGoCodeFromParams(t *testing.T) {
	testCases := []struct {
		structs []StructParameter
		result  string
	}{
		{structs: structsWithStruct, result: resultWithStruct},
		{structs: structsWithList, result: resultWithList},
		{structs: structsWithStructList, result: resultWithStructList},
		{structs: structsWithMap, result: resultWithMap},
		{structs: structWithInterface, result: resultWithInterface},
	}
	for _, tc := range testCases {
		actual, err := GenGoCodeFromParams(tc.structs)
		if err != nil {
			return
		}
		assert.Equal(t, tc.result, actual)
	}
}

var (
	defWithStruct = `
	parameter: {
		// +usage=Specify the mapping relationship between the http path and the workload port
		http: {
			path: int
		}
	}`
	defWithList = `
	parameter: {
		http: [string]: int
	}`
	defWithStructList = `
	parameter: {
		emptyDir?: [...{
				name:      string
				mountPath: string
				medium:    *"" | "Memory"
			}]
	}`
	defWithMap      = `parameter: [string]: string | null`
	defWithEmptyMap = `
	parameter: {
		data: {}
	}`

	structsWithStruct = []StructParameter{
		{
			Parameter: types.Parameter{
				Name:  "http",
				Type:  cue.StructKind,
				Usage: "Specify the mapping relationship between the http path and the workload port",
			},
			Fields: []Field{
				{
					Name:   "path",
					GoType: "int",
				},
			},
		},
		{
			Parameter: types.Parameter{
				Type: cue.StructKind,
				Name: "Parameter",
			},
			Fields: []Field{
				{
					Name:   "http",
					GoType: "HTTP",
				},
			},
		},
	}
	structsWithList = []StructParameter{
		{
			Parameter: types.Parameter{
				Type: cue.StructKind,
				Name: "Parameter",
			},
			Fields: []Field{
				{
					Name:   "http",
					GoType: "map[string]int",
				},
			},
		},
	}
	structsWithStructList = []StructParameter{
		{
			Parameter: types.Parameter{
				Type: cue.StructKind,
				Name: "emptyDir",
			},
			Fields: []Field{
				{Name: "name", GoType: "string"},
				{Name: "mountPath", GoType: "string"},
				{Name: "medium", GoType: "string"},
			},
		},
		{
			Parameter: types.Parameter{
				Type: cue.StructKind,
				Name: "Parameter",
			},
			Fields: []Field{
				{
					Name:   "emptyDir",
					GoType: "[]EmptyDir",
				},
			},
		},
	}
	structsWithMap = []StructParameter{
		{
			Parameter: types.Parameter{
				Type: cue.StructKind,
				Name: "Parameter",
			},
			GoType: "map[string]string",
			Fields: []Field{},
		},
	}
	structWithInterface = []StructParameter{
		{
			Parameter: types.Parameter{
				Type: cue.StructKind,
				Name: "Parameter",
			},
			GoType: "",
			Fields: []Field{
				{Name: "data", GoType: "map[string]interface{}"},
			},
		},
	}

	resultWithStruct     = "// HTTP Specify the mapping relationship between the http path and the workload port\ntype HTTP struct {\n\tPath int `json:\"path\"`\n}\n\n// Parameter -\ntype Parameter struct {\n\tHTTP HTTP `json:\"http\"`\n}\n"
	resultWithList       = "// Parameter -\ntype Parameter struct {\n\tHTTP map[string]int `json:\"http\"`\n}\n"
	resultWithStructList = "// EmptyDir -\ntype EmptyDir struct {\n\tName      string `json:\"name\"`\n\tMountPath string `json:\"mountPath\"`\n\tMedium    string `json:\"medium\"`\n}\n\n// Parameter -\ntype Parameter struct {\n\tEmptyDir []EmptyDir `json:\"emptyDir\"`\n}\n"
	resultWithMap        = "// Parameter -\ntype Parameter map[string]string"
	resultWithInterface  = "// Parameter -\ntype Parameter struct {\n\tData map[string]interface{} `json:\"data\"`\n}\n"
)

func TestGenAllDef(t *testing.T) {
	skipDefs := []string{
		// non-concrete structs like
		// foo: string|{secretRef: string}
		"container-image.cue",
		"export2config.cue",
		"webhook.cue",
		"notification.cue",
		"webhook-notification.cue",
		"env.cue",
		"command.cue",

		// not supported
		"json-merge-patch.cue",
		"json-patch.cue",

		// no args
		"apply-application.cue",
		"apply-application-in-parallel.cue",
	}
	glob, err := filepath.Glob("../../vela-templates/definitions/*/*.cue")
	assert.NoError(t, err)
	for _, f := range glob {
		if !stringInSlice(filepath.Base(f), skipDefs) {
			t.Run(filepath.Base(f), func(t *testing.T) {
				file, err := ioutil.ReadFile(f)
				assert.NoError(t, err)
				def := Definition{Unstructured: unstructured.Unstructured{}}
				err = def.FromCUEString(string(file), nil)
				assert.NoError(t, err)
				templateString, _, err := unstructured.NestedString(def.Object, DefinitionTemplateKeys...)
				assert.NoError(t, err)
				value, err := common.GetCUEParameterValue(templateString, nil)
				assert.NoError(t, err)
				_, err = GeneratorParameterStructs(value)
				assert.NoError(t, err)
			})
		}
	}
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
