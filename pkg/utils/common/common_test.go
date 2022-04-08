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

package common

import (
	"context"
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
)

var ResponseString = "Hello HTTP Get."

func TestInitBaseRestConfig(t *testing.T) {
	args, err := InitBaseRestConfig()
	assert.NotNil(t, t, args)
	assert.NoError(t, err)
}

func TestHTTPGet(t *testing.T) {
	type want struct {
		data   string
		errStr string
	}
	var ctx = context.Background()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, ResponseString)
	}))
	defer testServer.Close()

	cases := map[string]struct {
		reason string
		url    string
		want   want
	}{
		"normal case": {
			reason: "url is valid\n",
			url:    testServer.URL,
			want: want{
				data:   fmt.Sprintf("%s\n", ResponseString),
				errStr: "",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := HTTPGet(ctx, tc.url)
			if tc.want.errStr != "" {
				if diff := cmp.Diff(tc.want.errStr, err.Error(), test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nHTTPGet(...): -want error, +got error:\n%s", tc.reason, diff)
				}
			}

			if diff := cmp.Diff(tc.want.data, string(got)); diff != "" {
				t.Errorf("\n%s\nHTTPGet(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}

}

func TestGetCUEParameterValue(t *testing.T) {
	type want struct {
		err error
	}
	var validCueStr = `
parameter: {
	min: int
}
`

	var CueStrNotContainParameter = `
output: {
	min: int
}
`
	cases := map[string]struct {
		reason string
		cueStr string
		want   want
	}{
		"GetCUEParameterValue": {
			reason: "cue string is valid",
			cueStr: validCueStr,
			want: want{
				err: nil,
			},
		},
		"CUEStringNotContainParameter": {
			reason: "cue string doesn't contain Parameter",
			cueStr: CueStrNotContainParameter,
			want: want{
				err: fmt.Errorf("parameter not exist"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := GetCUEParameterValue(tc.cueStr, nil)
			if tc.want.err != nil {
				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want error, +got error:\n%s", tc.reason, diff)
				}
			}

		})
	}
}

func TestGetCUEParameterValue4RareCases(t *testing.T) {
	type want struct {
		errMsg string
	}

	var invalidCueStr = `
name
`
	cases := map[string]struct {
		reason string
		cueStr string
		want   want
	}{
		"CUEStringInvalid": {
			reason: "cue string is invalid",
			cueStr: invalidCueStr,
			want: want{
				errMsg: "reference \"name\" not found",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := GetCUEParameterValue(tc.cueStr, nil)
			if diff := cmp.Diff(tc.want.errMsg, err.Error(), test.EquateConditions()); diff != "" {
				t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

		})
	}
}

func TestGenOpenAPI(t *testing.T) {
	type want struct {
		targetSchemaFile string
		err              error
	}
	cases := map[string]struct {
		reason       string
		fileName     string
		targetSchema string
		want         want
	}{
		"GenOpenAPI": {
			reason:   "generate valid OpenAPI schema with context",
			fileName: "workload1.cue",
			want: want{
				targetSchemaFile: "workload1.json",
				err:              nil,
			},
		},
		"EmptyOpenAPI": {
			reason:   "generate empty OpenAPI schema",
			fileName: "emptyParameter.cue",
			want: want{
				targetSchemaFile: "emptyParameter.json",
				err:              nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			inst := cue.Build(load.Instances([]string{filepath.FromSlash(tc.fileName)}, &load.Config{
				Dir: "testdata",
			}))[0]
			got, err := GenOpenAPI(inst)
			if tc.want.err != nil {
				if diff := cmp.Diff(tc.want.err, errors.New(err.Error()), test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want error, +got error:\n%s", tc.reason, diff)
				}
			}
			if tc.want.targetSchemaFile == "" {
				return
			}
			wantSchema, _ := os.ReadFile(filepath.Join("testdata", tc.want.targetSchemaFile))
			if diff := cmp.Diff(wantSchema, got); diff != "" {
				t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestRealtimePrintCommandOutput(t *testing.T) {
	cmd := exec.Command("bash", "-c", "date")
	err := RealtimePrintCommandOutput(cmd, "")
	assert.NoError(t, err)
	cmd.Process.Kill()

	var logFile = "terraform.log"
	var hello = "Hello"
	cmd = exec.Command("bash", "-c", fmt.Sprintf("echo \"%s\"", hello))
	err = RealtimePrintCommandOutput(cmd, logFile)
	assert.NoError(t, err)

	data, _ := os.ReadFile(logFile)
	assert.Contains(t, string(data), hello)
	os.Remove(logFile)
}

func TestParseTerraformVariables(t *testing.T) {
	configuration := `
module "rds" {
  source = "terraform-alicloud-modules/rds/alicloud"
  engine = "MySQL"
  engine_version = "8.0"
  instance_type = "rds.mysql.c1.large"
  instance_storage = "20"
  instance_name = var.instance_name
  account_name = var.account_name
  password = var.password
}

output "DB_NAME" {
  value = module.rds.this_db_instance_name
}
output "DB_USER" {
  value = module.rds.this_db_database_account
}
output "DB_PORT" {
  value = module.rds.this_db_instance_port
}
output "DB_HOST" {
  value = module.rds.this_db_instance_connection_string
}
output "DB_PASSWORD" {
  value = module.rds.this_db_instance_port
}

variable "instance_name" {
  description = "RDS instance name"
  type = string
  default = "poc"
}

variable "account_name" {
  description = "RDS instance user account name"
  type = "string"
  default = "oam"
}

variable "password" {
  description = "RDS instance account password"
  type = "string"
  default = "xxx"
}

variable "intVar" {
  type = "number"
}

variable "boolVar" {
  type = "bool"
}

variable "listVar" {
  type = "list"
}

variable "mapVar" {
  type = "map"
}`

	variables, _, err := ParseTerraformVariables(configuration)
	assert.NoError(t, err)
	_, passwordExisted := variables["password"]
	assert.True(t, passwordExisted)

	_, intVarExisted := variables["password"]
	assert.True(t, intVarExisted)
}

func TestRefineParameterInstance(t *testing.T) {
	// test #parameter exists: mock issues in #1939 & #2062
	s := `parameter: #parameter
#parameter: {
	x?: string
	if x != "" {
	y: string
	}
}
patch: {
	if parameter.x != "" {
	label: parameter.x
	}
}`
	r := cue.Runtime{}
	inst, err := r.Compile("-", s)
	assert.NoError(t, err)
	_, err = RefineParameterInstance(inst)
	assert.NoError(t, err)
	// test #parameter not exist but parameter exists
	s = `parameter: {
	x?: string
	if x != "" {
	y: string
	}
}`
	inst, err = r.Compile("-", s)
	assert.NoError(t, err)
	_ = extractParameterDefinitionNodeFromInstance(inst)
	_, err = RefineParameterInstance(inst)
	assert.NoError(t, err)
	// test #parameter as int
	s = `parameter: #parameter
#parameter: int`
	inst, err = r.Compile("-", s)
	assert.NoError(t, err)
	_, err = RefineParameterInstance(inst)
	assert.NoError(t, err)
	// test invalid parameter kind
	s = `parameter: #parameter
#parameter: '\x03abc'`
	inst, err = r.Compile("-", s)
	assert.NoError(t, err)
	_, err = RefineParameterInstance(inst)
	assert.NotNil(t, err)
}

func TestFilterClusterObjectRefFromAddonObservability(t *testing.T) {
	ref := common.ClusterObjectReference{}
	ref.Name = AddonObservabilityGrafanaSvc
	ref.Namespace = types.DefaultKubeVelaNS
	resources := []common.ClusterObjectReference{ref}

	res := filterClusterObjectRefFromAddonObservability(resources)
	assert.Equal(t, 1, len(res))
	assert.Equal(t, "Service", res[0].Kind)
	assert.Equal(t, "v1", res[0].APIVersion)
}

func TestResourceNameClusterObjectReferenceFilter(t *testing.T) {
	fooRef := common.ClusterObjectReference{
		ObjectReference: corev1.ObjectReference{
			Name: "foo",
		}}
	barRef := common.ClusterObjectReference{
		ObjectReference: corev1.ObjectReference{
			Name: "bar",
		}}
	bazRef := common.ClusterObjectReference{
		ObjectReference: corev1.ObjectReference{
			Name: "baz",
		}}
	var refs = []common.ClusterObjectReference{
		fooRef, barRef, bazRef,
	}

	testCases := []struct {
		caseName     string
		filter       clusterObjectReferenceFilter
		filteredRefs []common.ClusterObjectReference
	}{
		{
			caseName:     "filter one resource",
			filter:       resourceNameClusterObjectReferenceFilter([]string{"foo"}),
			filteredRefs: []common.ClusterObjectReference{fooRef},
		},
		{
			caseName:     "not filter resources",
			filter:       resourceNameClusterObjectReferenceFilter([]string{}),
			filteredRefs: []common.ClusterObjectReference{fooRef, barRef, bazRef},
		},
		{
			caseName:     "filter multi resources",
			filter:       resourceNameClusterObjectReferenceFilter([]string{"foo", "bar"}),
			filteredRefs: []common.ClusterObjectReference{fooRef, barRef},
		},
	}
	for _, c := range testCases {
		filteredResource := filterResource(refs, c.filter)
		assert.Equal(t, c.filteredRefs, filteredResource, c.caseName)
	}
}

func TestRemoveEmptyString(t *testing.T) {
	withEmpty := []string{"foo", "bar", "", "baz", ""}
	noEmpty := removeEmptyString(withEmpty)
	assert.Equal(t, len(noEmpty), 3)
	for _, s := range noEmpty {
		assert.NotEmpty(t, s)
	}
}
