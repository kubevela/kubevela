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

package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

const TestDir = "testdata/definition"

func TestGetOpenAPISchema(t *testing.T) {
	type want struct {
		data string
		err  error
	}
	cases := map[string]struct {
		reason   string
		name     string
		fileDir  string
		fileName string
		want     want
	}{
		"PrepareANormalParameterCueFile": {
			reason:   "Prepare a normal parameter cue file",
			name:     "workload1",
			fileDir:  TestDir,
			fileName: "workload1.cue",
			want:     want{data: "{\"properties\":{\"min\":{\"title\":\"min\",\"type\":\"integer\"}},\"required\":[\"min\"],\"type\":\"object\"}", err: nil},
		},
		"CueFileNotContainParameter": {
			reason:   "Prepare a cue file which doesn't contain `parameter` section",
			name:     "invalidWorkload",
			fileDir:  TestDir,
			fileName: "workloadNoParameter.cue",
			want:     want{data: "", err: fmt.Errorf("capability invalidWorkload doesn't contain section `parameter`")},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			data, _ := ioutil.ReadFile(filepath.Join(tc.fileDir, tc.fileName))
			schematic := &common.Schematic{
				CUE: &common.CUE{
					Template: string(data),
				},
			}
			capability, _ := appfile.ConvertTemplateJSON2Object(tc.name, nil, schematic)
			schema, err := getOpenAPISchema(capability, pd)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetOpenAPISchema(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if tc.want.err == nil {
				assert.Equal(t, string(schema), tc.want.data)
			}
		})
	}
}

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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			swagger, _ := openapi3.NewSwaggerLoader().LoadSwaggerFromFile(filepath.Join(TestDir, tc.inputFile))
			schema := swagger.Components.Schemas[mycue.ParameterTag].Value
			fixOpenAPISchema("", schema)
			fixedSchema, _ := schema.MarshalJSON()
			expectedSchema, _ := ioutil.ReadFile(filepath.Join(TestDir, tc.fixedFile))
			assert.Equal(t, string(fixedSchema), string(expectedSchema))
		})
	}
}

func TestGenerateOpenAPISchemaFromCapabilityParameter(t *testing.T) {
	var invalidWorkloadName = "IAmAnInvalidWorkloadDefinition"
	capabilityDir, _ := system.GetCapabilityDir()
	if _, err := os.Stat(capabilityDir); err != nil && os.IsNotExist(err) {
		os.MkdirAll(capabilityDir, 0755)
	}

	type want struct {
		data []byte
		err  error
	}

	cases := map[string]struct {
		reason     string
		capability types.Capability
		want       want
	}{
		"GenerateOpenAPISchemaFromInvalidCapability": {
			reason:     "generate OpenAPI schema for an invalid Workload/Trait",
			capability: types.Capability{Name: invalidWorkloadName},
			want:       want{data: nil, err: fmt.Errorf("capability IAmAnInvalidWorkloadDefinition doesn't contain section `parameter`")},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := generateOpenAPISchemaFromCapabilityParameter(tc.capability, pd)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetDefinition(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.data, got); diff != "" {
				t.Errorf("\n%s\ngetDefinition(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
