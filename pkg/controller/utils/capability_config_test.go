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

package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestGetTerraformConfigurationFromRemote(t *testing.T) {
	type want struct {
		config string
		errMsg string
	}

	type args struct {
		name         string
		url          string
		path         string
		data         []byte
		variableFile string
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"valid": {
			args: args{
				name: "valid",
				url:  "https://github.com/kubevela-contrib/terraform-modules.git",
				path: "unittest/",
				data: []byte(`
variable "aaa" {
	type = list(object({
		type = string
		sourceArn = string
		config = string
	}))
	default = []
}`),
				variableFile: "main.tf",
			},
			want: want{
				config: `
variable "aaa" {
	type = list(object({
		type = string
		sourceArn = string
		config = string
	}))
	default = []
}`,
			},
		},
		"configuration is remote with path": {
			args: args{
				name: "aws-subnet",
				url:  "https://github.com/kubevela-contrib/terraform-modules.git",
				path: "unittest/aws/subnet",
				data: []byte(`
variable "aaa" {
	type = list(object({
		type = string
		sourceArn = string
		config = string
	}))
	default = []
}`),
				variableFile: "variables.tf",
			},
			want: want{
				config: `
variable "aaa" {
	type = list(object({
		type = string
		sourceArn = string
		config = string
	}))
	default = []
}`,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			home, _ := os.UserHomeDir()
			path := filepath.Join(home, ".vela", "terraform")
			tmpPath := filepath.Join(path, tc.args.name, tc.args.path)
			if len(tc.args.data) > 0 {
				err := os.MkdirAll(tmpPath, os.ModePerm)
				assert.NilError(t, err)
				err = os.WriteFile(filepath.Clean(filepath.Join(tmpPath, tc.args.variableFile)), tc.args.data, 0644)
				assert.NilError(t, err)
			}
			defer os.RemoveAll(tmpPath)

			conf, err := GetTerraformConfigurationFromRemote(tc.args.name, tc.args.url, tc.args.path, nil)
			if tc.want.errMsg != "" {
				if err != nil && !strings.Contains(err.Error(), tc.want.errMsg) {
					t.Errorf("\n%s\nGetTerraformConfigurationFromRemote(...): -want error %v, +got error:%s", name, err, tc.want.errMsg)
				}
			} else {
				assert.Equal(t, tc.want.config, conf)
			}
		})
	}
}
