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

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
)

var RefTestDir = filepath.Join(TestDir, "ref")

func TestCreateRefTestDir(t *testing.T) {
	if _, err := os.Stat(RefTestDir); err != nil && os.IsNotExist(err) {
		err := os.MkdirAll(RefTestDir, 0750)
		assert.NoError(t, err)
	}
}

func TestCreateMarkdown(t *testing.T) {
	workloadName := "workload1"
	traitName := "trait1"
	scopeName := "scope1"

	workloadCueTemplate := `
parameter: {
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string
}
`
	traitCueTemplate := `
parameter: {
	replicas: int
}
`

	cases := map[string]struct {
		reason       string
		capabilities []types.Capability
		want         error
	}{
		"WorkloadTypeAndTraitCapability": {
			reason: "valid capabilities",
			capabilities: []types.Capability{
				{
					Name:        workloadName,
					Type:        types.TypeWorkload,
					CueTemplate: workloadCueTemplate,
					Category:    types.CUECategory,
				},
				{
					Name:        traitName,
					Type:        types.TypeTrait,
					CueTemplate: traitCueTemplate,
					Category:    types.CUECategory,
				},
			},
			want: nil,
		},
		"ScopeTypeCapability": {
			reason: "invalid capabilities",
			capabilities: []types.Capability{
				{
					Name: scopeName,
					Type: types.TypeScope,
				},
			},
			want: fmt.Errorf("the type of the capability is not right"),
		},
	}
	ref := &MarkdownReference{}
	ctx := context.Background()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := ref.CreateMarkdown(ctx, tc.capabilities, RefTestDir, ReferenceSourcePath)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nCreateMakrdown(...): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}

}

func TestPrepareParameterTable(t *testing.T) {
	ref := MarkdownReference{}
	tableName := "hello"
	var depth int = 1
	parameterList := []ReferenceParameter{
		{
			PrintableType: "string",
			Depth:         &depth,
		},
	}
	parameterName := "cpu"
	parameterList[0].Name = parameterName
	parameterList[0].Required = true
	refContent := ref.prepareParameter(tableName, parameterList, types.CUECategory)
	assert.Contains(t, refContent, parameterName)
	assert.Contains(t, refContent, "cpu")
}

func TestDeleteRefTestDir(t *testing.T) {
	if _, err := os.Stat(RefTestDir); err == nil {
		err := os.RemoveAll(RefTestDir)
		assert.NoError(t, err)
	}
}

func TestWalkParameterSchema(t *testing.T) {
	testcases := []struct {
		data       string
		ExpectRefs map[string]map[string]ReferenceParameter
	}{
		{
			data: `{
    "properties": {
        "cmd": {
            "description": "Commands to run in the container", 
            "items": {
                "type": "string"
            }, 
            "title": "cmd", 
            "type": "array"
        }, 
        "image": {
            "description": "Which image would you like to use for your service", 
            "title": "image", 
            "type": "string"
        }
    }, 
    "required": [
        "image"
    ], 
    "type": "object"
}`,
			ExpectRefs: map[string]map[string]ReferenceParameter{
				"# Properties": {
					"cmd": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "cmd",
							Usage:    "Commands to run in the container",
							JSONType: "array",
						},
						PrintableType: "array",
					},
					"image": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "image",
							Required: true,
							Usage:    "Which image would you like to use for your service",
							JSONType: "string",
						},
						PrintableType: "string",
					},
				},
			},
		},
		{
			data: `{
    "properties": { 
        "obj": {
            "properties": {
                "f0": {
                    "default": "v0", 
                    "type": "string"
                }, 
                "f1": {
                    "default": "v1", 
                    "type": "string"
                }, 
                "f2": {
                    "default": "v2", 
                    "type": "string"
                }
            }, 
            "type": "object"
        },
    }, 
    "type": "object"
}`,
			ExpectRefs: map[string]map[string]ReferenceParameter{
				"# Properties": {
					"obj": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "obj",
							JSONType: "object",
						},
						PrintableType: "[obj](#obj)",
					},
				},
				"## obj": {
					"f0": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f0",
							Default:  "v0",
							JSONType: "string",
						},
						PrintableType: "string",
					},
					"f1": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f1",
							Default:  "v1",
							JSONType: "string",
						},
						PrintableType: "string",
					},
					"f2": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f2",
							Default:  "v2",
							JSONType: "string",
						},
						PrintableType: "string",
					},
				},
			},
		},
		{
			data: `{
    "properties": {
        "obj": {
            "properties": {
                "f0": {
                    "default": "v0", 
                    "type": "string"
                }, 
                "f1": {
                    "default": "v1", 
                    "type": "object", 
                    "properties": {
                        "g0": {
                            "default": "v2", 
                            "type": "string"
                        }
                    }
                }
            }, 
            "type": "object"
        }
    }, 
    "type": "object"
}`,
			ExpectRefs: map[string]map[string]ReferenceParameter{
				"# Properties": {
					"obj": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "obj",
							JSONType: "object",
						},
						PrintableType: "[obj](#obj)",
					},
				},
				"## obj": {
					"f0": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f0",
							Default:  "v0",
							JSONType: "string",
						},
						PrintableType: "string",
					},
					"f1": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f1",
							Default:  "v1",
							JSONType: "object",
						},
						PrintableType: "[f1](#f1)",
					},
				},
				"### f1": {
					"g0": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "g0",
							Default:  "v2",
							JSONType: "string",
						},
						PrintableType: "string",
					},
				},
			},
		},
	}
	for _, cases := range testcases {
		helmRefs = make([]HELMReference, 0)
		parameterJSON := fmt.Sprintf(BaseOpenAPIV3Template, cases.data)
		swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(json.RawMessage(parameterJSON))
		assert.Equal(t, nil, err)
		parameters := swagger.Components.Schemas["parameter"].Value
		WalkParameterSchema(parameters, "Properties", 0)
		refs := make(map[string]map[string]ReferenceParameter)
		for _, items := range helmRefs {
			refs[items.Name] = make(map[string]ReferenceParameter)
			for _, item := range items.Parameters {
				refs[items.Name][item.Name] = item
			}
		}
		assert.Equal(t, true, reflect.DeepEqual(cases.ExpectRefs, refs))
	}
}
