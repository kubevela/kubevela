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

package docgen

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

func TestGenerateCUETemplateProperties(t *testing.T) {
	// Read componentDef for a valid capability
	componentDefYAML, err := os.ReadFile("testdata/componentDef.yaml")
	require.NoError(t, err)
	var componentDef v1beta1.ComponentDefinition
	err = yaml.Unmarshal(componentDefYAML, &componentDef)
	require.NoError(t, err)

	// Define a struct to unmarshal the raw extension
	type extensionSpec struct {
		Template string `json:"template"`
	}
	var extSpec extensionSpec
	err = yaml.Unmarshal(componentDef.Spec.Extension.Raw, &extSpec)
	require.NoError(t, err)

	// Define test cases
	testCases := []struct {
		name           string
		capability     *types.Capability
		expectedTables int
		expectErr      bool
	}{
		{
			name: "valid component definition",
			capability: &types.Capability{
				Name:        "test-component",
				CueTemplate: extSpec.Template,
			},
			expectedTables: 2,
			expectErr:      false,
		},
		{
			name: "invalid cue template",
			capability: &types.Capability{
				Name:        "invalid-cue",
				CueTemplate: `parameter: { image: }`,
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ref := &ConsoleReference{}
			doc, console, err := ref.GenerateCUETemplateProperties(tc.capability)

			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, doc)
			require.Len(t, console, tc.expectedTables)
		})
	}
}

func TestGenerateTerraformCapabilityProperties(t *testing.T) {
	ref := &ConsoleReference{}
	type args struct {
		cap types.Capability
	}

	type want struct {
		tableName1 string
		tableName2 string
		errMsg     string
	}
	testcases := map[string]struct {
		args args
		want want
	}{
		"normal": {
			args: args{
				cap: types.Capability{
					TerraformConfiguration: `
resource "alicloud_oss_bucket" "bucket-acl" {
  bucket = var.bucket
  acl = var.acl
}

output "BUCKET_NAME" {
  value = "${alicloud_oss_bucket.bucket-acl.bucket}.${alicloud_oss_bucket.bucket-acl.extranet_endpoint}"
}

variable "bucket" {
  description = "OSS bucket name"
  default = "vela-website"
  type = string
}

variable "acl" {
  description = "OSS bucket ACL, supported 'private', 'public-read', 'public-read-write'"
  default = "private"
  type = string
}
`,
				},
			},
			want: want{
				errMsg:     "",
				tableName1: "",
				tableName2: "#### writeConnectionSecretToRef",
			},
		},
		"configuration is in git remote": {
			args: args{
				cap: types.Capability{
					Name:                   "ecs",
					TerraformConfiguration: "https://github.com/wonderflow/terraform-alicloud-ecs-instance.git",
					ConfigurationType:      "remote",
				},
			},
			want: want{
				errMsg:     "",
				tableName1: "",
				tableName2: "#### writeConnectionSecretToRef",
			},
		},
		"configuration is not valid": {
			args: args{
				cap: types.Capability{
					TerraformConfiguration: `abc`,
				},
			},
			want: want{
				errMsg: "failed to generate capability properties: :1,1-4: Argument or block definition required; An " +
					"argument or block definition is required here. To set an argument, use the equals sign \"=\" to " +
					"introduce the argument value.",
			},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			consoleRef, err := ref.GenerateTerraformCapabilityProperties(tc.args.cap)
			var errMsg string
			if err != nil {
				errMsg = err.Error()
				if diff := cmp.Diff(tc.want.errMsg, errMsg); diff != "" {
					t.Errorf("\n%s\nGenerateTerraformCapabilityProperties(...): -want error, +got error:\n%s\n", name, diff)
				}
			} else {
				if diff := cmp.Diff(2, len(consoleRef)); diff != "" {
					t.Errorf("\n%s\nGenerateTerraformCapabilityProperties(...): -want, +got:\n%s\n", name, diff)
				}
				if diff := cmp.Diff(tc.want.tableName1, consoleRef[0].TableName); diff != "" {
					t.Errorf("\n%s\nGenerateTerraformCapabilityProperties(...): -want, +got:\n%s\n", name, diff)
				}
				if diff := cmp.Diff(tc.want.tableName2, consoleRef[1].TableName); diff != "" {
					t.Errorf("\n%s\nGenerateTerraformCapabilityProperties(...): -want, +got:\n%s\n", name, diff)
				}
			}
		})
	}
}
