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
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/agiledragon/gomonkey/v2"

	"github.com/pkg/errors"
	"gopkg.in/src-d/go-git.v4"
	"gotest.tools/assert"
)

func TestGetTerraformConfigurationFromRemote(t *testing.T) {
	// If you hit a panic on macOS as below, please fix it by referencing https://github.com/eisenxp/macos-golink-wrapper.
	// panic: permission denied [recovered]
	//    panic: permission denied
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
		// mockWorkingPath will create `/tmp/terraform`
		mockWorkingPath bool
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"valid": {
			args: args{
				name: "valid",
				url:  "https://github.com/kubevela-contrib/terraform-modules.git",
				path: "",
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
				path: "aws/subnet",
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
		"working path exists": {
			args: args{
				variableFile:    "main.tf",
				mockWorkingPath: true,
			},
			want: want{
				errMsg: "failed to remove the directory",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.args.mockWorkingPath {
				err := os.MkdirAll("./tmp/terraform", 0755)
				assert.NilError(t, err)
				defer os.RemoveAll("./tmp/terraform")
				patch1 := ApplyFunc(os.Remove, func(_ string) error {
					return errors.New("failed")
				})
				defer patch1.Reset()
				patch2 := ApplyFunc(os.Open, func(_ string) (*os.File, error) {
					return nil, errors.New("failed")
				})
				defer patch2.Reset()
			}

			patch := ApplyFunc(git.PlainCloneContext, func(ctx context.Context, path string, isBare bool, o *git.CloneOptions) (*git.Repository, error) {
				var tmpPath string
				if tc.args.path != "" {
					tmpPath = filepath.Join("./tmp/terraform", tc.args.name, tc.args.path)
				} else {
					tmpPath = filepath.Join("./tmp/terraform", tc.args.name)
				}
				err := os.MkdirAll(tmpPath, os.ModePerm)
				assert.NilError(t, err)
				err = ioutil.WriteFile(filepath.Clean(filepath.Join(tmpPath, tc.args.variableFile)), tc.args.data, 0644)
				assert.NilError(t, err)
				return nil, nil
			})
			defer patch.Reset()

			conf, err := GetTerraformConfigurationFromRemote(tc.args.name, tc.args.url, tc.args.path)
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
