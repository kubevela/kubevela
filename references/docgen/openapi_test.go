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

package docgen

import (
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestGenerateConsoleDocument(t *testing.T) {
	testCases := []struct {
		name       string
		title      string
		schema     *openapi3.Schema
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "empty schema",
			title:      "Test",
			schema:     &openapi3.Schema{},
			wantOutput: "",
		},
		{
			name:  "simple schema",
			title: "",
			schema: &openapi3.Schema{
				Properties: map[string]*openapi3.SchemaRef{
					"name": {
						Value: &openapi3.Schema{
							Title:       "name",
							Description: "The name of the resource.",
							Type:        &openapi3.Types{openapi3.TypeString},
						},
					},
					"port": {
						Value: &openapi3.Schema{
							Title:       "port",
							Description: "The port to expose.",
							Type:        &openapi3.Types{openapi3.TypeInteger},
						},
					},
				},
			},
			wantOutput: `
+------+---------+---------------------------+----------+---------+---------+
| NAME |  TYPE   |        DESCRIPTION        | REQUIRED | OPTIONS | DEFAULT |
+------+---------+---------------------------+----------+---------+---------+
| name | string  | The name of the resource. | false    |         |         |
| port | integer | The port to expose.       | false    |         |         |
+------+---------+---------------------------+----------+---------+---------+
`,
		},
		{
			name:  "nested schema",
			title: "parent",
			schema: &openapi3.Schema{
				Required: []string{"child"},
				Properties: map[string]*openapi3.SchemaRef{
					"child": {
						Value: &openapi3.Schema{
							Title: "child",
							Type:  &openapi3.Types{openapi3.TypeObject},
							Properties: map[string]*openapi3.SchemaRef{
								"leaf": {
									Value: &openapi3.Schema{
										Title: "leaf",
										Type:  &openapi3.Types{openapi3.TypeString},
									},
								},
							},
						},
					},
				},
			},
			wantOutput: `parent
+----------------+--------+-------------+----------+---------+---------+
|      NAME      |  TYPE  | DESCRIPTION | REQUIRED | OPTIONS | DEFAULT |
+----------------+--------+-------------+----------+---------+---------+
| (parent).child | object |             | true     |         |         |
+----------------+--------+-------------+----------+---------+---------+
parent.child
+---------------------+--------+-------------+----------+---------+---------+
|        NAME         |  TYPE  | DESCRIPTION | REQUIRED | OPTIONS | DEFAULT |
+---------------------+--------+-------------+----------+---------+---------+
| (parent.child).leaf | string |             | false    |         |         |
+---------------------+--------+-------------+----------+---------+---------+
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := GenerateConsoleDocument(tc.title, tc.schema)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			// Trim whitespace for consistent comparison and sort lines to avoid flakiness
			expectedLines := strings.Split(strings.TrimSpace(tc.wantOutput), "\n")
			actualLines := strings.Split(strings.TrimSpace(doc), "\n")
			sort.Strings(expectedLines)
			sort.Strings(actualLines)
			require.Equal(t, expectedLines, actualLines)
		})
	}
}
