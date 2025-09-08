/*
Copyright 2022 The KubeVela Authors.

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

package schema

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const TestDir = "testdata"

func TestFixOpenAPISchema(t *testing.T) {
	cases := map[string]struct {
		inputFile string
		fixedFile string
	}{
		"StandardWorkload": {
			inputFile: "webservice.json",
			fixedFile: "webserviceFixed.json",
		},
		"ShortTagJson": {
			inputFile: "shortTagSchema.json",
			fixedFile: "shortTagSchemaFixed.json",
		},
		"EmptyArrayJson": {
			inputFile: "arrayWithoutItemsSchema.json",
			fixedFile: "arrayWithoutItemsSchemaFixed.json",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			swagger, _ := openapi3.NewLoader().LoadFromFile(filepath.Join(TestDir, tc.inputFile))
			schema := swagger.Components.Schemas[process.ParameterFieldName].Value
			FixOpenAPISchema("", schema)
			fixedSchema, _ := schema.MarshalJSON()
			expectedSchema, _ := os.ReadFile(filepath.Join(TestDir, tc.fixedFile))
			assert.Equal(t, string(fixedSchema), string(expectedSchema))
		})
	}
}

func TestParsePropertiesToSchema(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		cue       string
		path      []string
		wantErr   bool
		checkFunc func(t *testing.T, schema *openapi3.Schema)
	}{
		{
			name: "happy path no path",
			cue: `parameter: {
				name: string
				age: int
			}`,
			wantErr: false,
			checkFunc: func(t *testing.T, schema *openapi3.Schema) {
				r := assert.New(t)
				r.NotNil(schema)
				r.Contains(schema.Properties, "name")
				r.Contains(schema.Properties, "age")
				r.Equal("string", (*schema.Properties["name"].Value.Type)[0])
				r.Equal("integer", (*schema.Properties["age"].Value.Type)[0])
			},
		},
		{
			name: "happy path with path",
			cue: `
			template: {
				parameter: {
					address: string
				}
			}`,
			path:    []string{"template"},
			wantErr: false,
			checkFunc: func(t *testing.T, schema *openapi3.Schema) {
				r := assert.New(t)
				r.NotNil(schema)
				r.Contains(schema.Properties, "address")
				r.Len(schema.Properties, 1)
			},
		},
		{
			name:    "invalid cue string",
			cue:     `parameter: { name: }`,
			wantErr: true,
		},
		{
			name:    "invalid path",
			cue:     `parameter: { name: string }`,
			path:    []string{"bad-path"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema, err := ParsePropertiesToSchema(ctx, tc.cue, tc.path...)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tc.checkFunc != nil {
					tc.checkFunc(t, schema)
				}
			}
		})
	}
}

func TestConvertOpenAPISchema2SwaggerObject(t *testing.T) {
	cases := []struct {
		name      string
		jsonData  []byte
		wantErr   bool
		errString string
		checkFunc func(*testing.T, *openapi3.Schema)
	}{
		{
			name: "happy path",
			jsonData: []byte(`{
				"openapi": "3.0.0",
				"info": { "title": "My API", "version": "1.0.0" },
				"paths": {},
				"components": {
					"schemas": {
						"parameter": {
							"type": "object",
							"properties": { "name": { "type": "string" } }
						}
					}
				}
			}`),
			wantErr: false,
			checkFunc: func(t *testing.T, schema *openapi3.Schema) {
				assert.NotNil(t, schema)
				assert.Contains(t, schema.Properties, "name")
			},
		},
		{
			name:     "invalid json",
			jsonData: []byte(`{"bad"`),
			wantErr:  true,
		},
		{
			name: "missing parameter schema",
			jsonData: []byte(`{
				"openapi": "3.0.0",
				"info": { "title": "My API", "version": "1.0.0" },
				"paths": {},
				"components": { "schemas": { "other": {} } }
			}`),
			wantErr:   true,
			errString: util.ErrGenerateOpenAPIV2JSONSchemaForCapability,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema, err := ConvertOpenAPISchema2SwaggerObject(tc.jsonData)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errString != "" {
					assert.EqualError(t, err, tc.errString)
				}
			} else {
				assert.NoError(t, err)
				if tc.checkFunc != nil {
					tc.checkFunc(t, schema)
				}
			}
		})
	}
}
