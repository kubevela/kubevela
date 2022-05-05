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

package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
)

func TestGenerateSchemaFromValues(t *testing.T) {
	testValues, err := os.ReadFile("./testdata/values.yaml")
	if err != nil {
		t.Error(err, "cannot load test data")
	}
	wantSchema, err := os.ReadFile("./testdata/values.schema.json")
	if err != nil {
		t.Error(err, "cannot load expected data")
	}
	wantSchemaMap := map[string]interface{}{}
	// convert bytes to map for diff converience
	_ = json.Unmarshal(wantSchema, &wantSchemaMap)
	result, err := generateSchemaFromValues(testValues)
	if err != nil {
		t.Error(err, "failed generate schema from values")
	}
	resultMap := map[string]interface{}{}
	if err := json.Unmarshal(result, &resultMap); err != nil {
		t.Error(err, "cannot unmarshal result bytes")
	}
	if diff := cmp.Diff(resultMap, wantSchemaMap); diff != "" {
		t.Fatalf("\ngenerateSchemaFromValues(...)(...) -want +get \n%s", diff)
	}
}

func TestGetChartValuesJSONSchema(t *testing.T) {
	testHelm := testData("podinfo", "5.1.4", "https://charts.kubevela.net/example", "testSecret")
	wantSchema, err := os.ReadFile("./testdata/podinfo.values.schema.json")
	if err != nil {
		t.Error(err, "cannot load expected data")
	}
	wantSchemaMap := map[string]interface{}{}
	// convert bytes to map for diff converience
	_ = json.Unmarshal(wantSchema, &wantSchemaMap)
	result, err := GetChartValuesJSONSchema(context.Background(), testHelm)
	if err != nil {
		t.Error(err, "failed get schema")
	}
	resultMap := map[string]interface{}{}
	if err := json.Unmarshal(result, &resultMap); err != nil {
		t.Error(err, "cannot unmarshal result bytes")
	}
	if diff := cmp.Diff(resultMap, wantSchemaMap); diff != "" {
		t.Fatalf("\nGetChartValuesJSONSchema(...)(...) -want +get \n%s", diff)
	}
}

func TestChangeEnumToDefault(t *testing.T) {
	// testData contains object, string, integer, bool, and array type fields
	// with enum and required values
	testData := `{"properties":{"array":{"enum":[["a","b","c"]],"items":{"type":"string"},"type":"array"},"bool":{"enum":[false],"type":"boolean"},"integer":{"enum":[1],"type":"integer"},"obj":{"properties":{"f0":{"enum":["v0"],"type":"string"},"f1":{"enum":["v1"],"type":"string"},"f2":{"enum":["v2"],"type":"string"}},"required":["f0","f1","f2"],"type":"object"},"string":{"enum":["a"],"type":"string"}},"required":["bool","string","obj","array","integer"],"type":"object"}`

	s := fmt.Sprintf(`{"components":{"schemas":{"values":%s}}}`, testData)
	testSwagger, err := openapi3.NewLoader().LoadFromData([]byte(s))
	if err != nil {
		t.Error(err)
	}
	testSchema := testSwagger.Components.Schemas["values"].Value
	changeEnumToDefault(testSchema)
	result, err := testSchema.MarshalJSON()
	if err != nil {
		t.Error(err)
	}
	resultMap := map[string]interface{}{}
	err = json.Unmarshal(result, &resultMap)
	if err != nil {
		t.Error(err)
	}
	want := `{"properties":{"array":{"default":["a","b","c"],"items":{"type":"string"},"type":"array"},"bool":{"default":false,"type":"boolean"},"integer":{"default":1,"type":"integer"},"obj":{"properties":{"f0":{"default":"v0","type":"string"},"f1":{"default":"v1","type":"string"},"f2":{"default":"v2","type":"string"}},"type":"object"},"string":{"default":"a","type":"string"}},"type":"object"}`
	wantMap := map[string]interface{}{}
	_ = json.Unmarshal([]byte(want), &wantMap)
	if diff := cmp.Diff(resultMap, wantMap); diff != "" {
		t.Fatalf("\nchangeEnumToDefault(...) -want +get %s\n", diff)
	}
}

func TestMakeSwaggerCompatible(t *testing.T) {
	tests := []struct {
		caseName string
		testdata string
		want     string
	}{
		{
			caseName: "integer type array",
			testdata: `{"integerArray": {
    "type": "array",
    "items": [
      {
        "type": "integer",
        "enum": [
          0
        ]
      },
      {
        "type": "integer",
        "enum": [
          1
        ]
      },
      {
        "type": "integer",
        "enum": [
          2
        ]
      }
    ],
    "enum": [
      [
        0,
        1,
        2
      ]
    ]
  }}`,
			want: `{"integerArray":{"enum":[[0,1,2]],"items":{"enum":null,"type":"integer"},"type":"array"}}`,
		},
		{
			caseName: "string type array",
			testdata: `{"stringArray": {
  "type": "array",
  "items": [
    {
      "type": "string",
      "enum": [
        "a"
      ]
    },
    {
      "type": "string",
      "enum": [
        "b"
      ]
    },
    {
      "type": "string",
      "enum": [
        "c"
      ]
    }
  ],
  "enum": [
    [
      "a",
      "b",
      "c"
    ]
  ]
}}`,
			want: `{"stringArray":{"enum":[["a","b","c"]],"items":{"enum":null,"type":"string"},"type":"array"}}`,
		},
		{
			caseName: "bool type array",
			testdata: `{"boolArray": {
  "type": "array",
  "items": [
    {
      "type": "boolean",
      "enum": [
        true
      ]
    },
    {
      "type": "boolean",
      "enum": [
        false
      ]
    }
  ],
  "enum": [
    [
      true,
      false
    ]
  ]
}}`,
			want: `{"boolArray":{"enum":[[true,false]],"items":{"enum":null,"type":"boolean"},"type":"array"}}`,
		},
		{
			caseName: "object type array",
			testdata: `{"objectArray": {
  "type": "array",
  "items": [
    {
      "type": "object",
      "required": [
        "f0",
        "f1",
        "f2"
      ],
      "properties": {
        "f0": {
          "type": "string",
          "enum": [
            "v0"
          ]
        },
        "f1": {
          "type": "string",
          "enum": [
            "v1"
          ]
        },
        "f2": {
          "type": "string",
          "enum": [
            "v2"
          ]
        }
      }
    }
  ]
}}`,
			want: `{"objectArray":{"items":{"enum":null,"properties":{"f0":{"enum":["v0"],"type":"string"},"f1":{"enum":["v1"],"type":"string"},"f2":{"enum":["v2"],"type":"string"}},"required":["f0","f1","f2"],"type":"object"},"type":"array"}}`,
		},
		{
			caseName: "object type array embeds object type array",
			testdata: `{
  "objectArray": {
    "type": "array",
    "items": [
      {
        "type": "array",
        "items": [
          {
            "type": "object",
            "required": [
              "f0"
            ],
            "properties": {
              "f0": {
                "type": "string",
                "enum": [
                  "v0"
                ]
              }
            }
          }
        ]
      }
    ]
  }
}
`,
			want: `{"objectArray":{"items":{"enum":null,"items":{"enum":null,"properties":{"f0":{"enum":["v0"],"type":"string"}},"required":["f0"],"type":"object"},"type":"array"},"type":"array"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.caseName, func(t *testing.T) {
			result, err := makeSwaggerCompatible([]byte(tc.testdata))
			if err != nil {
				t.Error(err)
			}
			resultMap := map[string]interface{}{}
			err = json.Unmarshal(result, &resultMap)
			if err != nil {
				t.Error(err)
			}
			wantMap := map[string]interface{}{}
			_ = json.Unmarshal([]byte(tc.want), &wantMap)
			if diff := cmp.Diff(resultMap, wantMap); diff != "" {
				t.Fatalf("\nmakeSwaggerCompatible(...) -want +get %s\n", diff)
			}
		})
	}
}
