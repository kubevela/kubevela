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
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue/load"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
)

var ResponseString = "Hello HTTP Get."

func TestInitBaseRestConfig(t *testing.T) {
	args, err := InitBaseRestConfig()
	assert.NotNil(t, t, args)
	assert.NoError(t, err, "you need to have a kubeconfig in your Environment")
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
			got, err := HTTPGetWithOption(ctx, tc.url, nil)
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

func TestHTTPGetWithOption(t *testing.T) {
	type want struct {
		data string
	}
	var ctx = context.Background()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok {
			w.Write([]byte("Error parsing basic auth"))
			w.WriteHeader(401)
			return
		}
		if u != "test-user" {
			w.Write([]byte(fmt.Sprintf("Username provided is incorrect: %s", u)))
			w.WriteHeader(401)
			return
		}
		if p != "test-pass" {
			w.Write([]byte(fmt.Sprintf("Password provided is incorrect: %s", p)))
			w.WriteHeader(401)
			return
		}
		w.Write([]byte("correct password"))
		w.WriteHeader(200)
	}))
	defer testServer.Close()

	cases := map[string]struct {
		opts *HTTPOption
		url  string
		want want
	}{
		"without auth case": {
			opts: nil,
			url:  testServer.URL,
			want: want{
				data: "Error parsing basic auth",
			},
		},
		"error user name case": {
			opts: &HTTPOption{
				Username: "no-user",
				Password: "test-pass",
			},
			url: testServer.URL,
			want: want{
				data: "Username provided is incorrect: no-user",
			},
		},
		"error password case": {
			opts: &HTTPOption{
				Username: "test-user",
				Password: "error-pass",
			},
			url: testServer.URL,
			want: want{
				data: "Password provided is incorrect: error-pass",
			},
		},
		"correct password case": {
			opts: &HTTPOption{
				Username: "test-user",
				Password: "test-pass",
			},
			url: testServer.URL,
			want: want{
				data: "correct password",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := HTTPGetWithOption(ctx, tc.url, tc.opts)
			assert.NoError(t, err)

			if diff := cmp.Diff(tc.want.data, string(got)); diff != "" {
				t.Errorf("\n%s\nHTTPGet(...): -want, +got:\n%s", tc.want.data, diff)
			}
		})
	}

}

func TestHttpGetCaFile(t *testing.T) {
	type want struct {
		data string
	}
	var ctx = context.Background()
	testServer := &http.Server{Addr: ":10443"}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("this is https server"))
		writer.WriteHeader(200)
	})

	go func() {
		err := testServer.ListenAndServeTLS("./testdata/server.crt", "./testdata/server.key")
		assert.NoError(t, err)
	}()
	time.Sleep(time.Millisecond)
	caFile, err := os.ReadFile("./testdata/server.crt")
	assert.NoError(t, err)

	cases := map[string]struct {
		opts *HTTPOption
		url  string
		want want
	}{
		"with caFile": {
			opts: &HTTPOption{CaFile: string(caFile)},
			url:  "https://127.0.0.1:10443",
			want: want{
				data: "this is https server",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := HTTPGetWithOption(ctx, tc.url, tc.opts)
			assert.NoError(t, err)

			if diff := cmp.Diff(tc.want.data, string(got)); diff != "" {
				t.Errorf("\n%s\nHTTPGet(...): -want, +got:\n%s", tc.want.data, diff)
			}
		})
	}
}

func TestHttpGetForbidRedirect(t *testing.T) {
	var ctx = context.Background()
	testServer := &http.Server{Addr: ":19090"}

	http.HandleFunc("/redirect", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "http://192.168.1.1", http.StatusFound)
	})

	go func() {
		err := testServer.ListenAndServe()
		assert.NoError(t, err)
	}()
	time.Sleep(time.Millisecond)

	_, err := HTTPGetWithOption(ctx, "http://127.0.0.1:19090/redirect", nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "got a redirect response which is forbidden"))
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

	cases := map[string]struct {
		reason string
		cueStr string
		want   want
	}{
		"CUEParameterNotFound": {
			reason: "cue parameter not found",
			cueStr: `name: string`,
			want: want{
				errMsg: "parameter not exist",
			},
		},
		"CUEStringInvalid": {
			reason: "cue string is invalid",
			cueStr: `name`,
			want: want{
				errMsg: "parameter not exist",
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
			instances := load.Instances([]string{filepath.FromSlash(tc.fileName)}, &load.Config{
				Dir: "testdata",
			})
			val, err := value.NewValueWithInstance(instances[0], nil, "")
			assert.NoError(t, err)
			got, err := GenOpenAPI(val)
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
	val, err := value.NewValue(s, nil, "")
	assert.NoError(t, err)
	assert.NoError(t, val.CueValue().Err())
	_, err = RefineParameterValue(val)
	assert.NoError(t, err)
	// test #parameter not exist but parameter exists
	s = `parameter: {
	x?: string
	if x != "" {
	y: string
	}
}`
	val, err = value.NewValue(s, nil, "")
	assert.NoError(t, err)
	assert.NoError(t, val.CueValue().Err())
	assert.NoError(t, err)
	_, err = RefineParameterValue(val)
	assert.NoError(t, err)
	// test #parameter as int
	s = `parameter: #parameter
#parameter: int`
	val, err = value.NewValue(s, nil, "")
	assert.NoError(t, err)
	assert.NoError(t, val.CueValue().Err())
	_, err = RefineParameterValue(val)
	assert.NoError(t, err)
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

func TestHTTPGetKubernetesObjects(t *testing.T) {
	_, err := HTTPGetKubernetesObjects(context.Background(), "invalid-url")
	assert.NotNil(t, err)
	uns, err := HTTPGetKubernetesObjects(context.Background(), "https://gist.githubusercontent.com/Somefive/b189219a9222eaa70b8908cf4379402b/raw/920e83b1a2d56b584f9d8c7a97810a505a0bbaad/example-busybox-resources.yaml")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(uns))
	assert.Equal(t, "busybox", uns[0].GetName())
	assert.Equal(t, "Deployment", uns[0].GetKind())
	assert.Equal(t, "busybox", uns[1].GetName())
	assert.Equal(t, "ConfigMap", uns[1].GetKind())
}

func TestGetRawConfig(t *testing.T) {
	assert.NoError(t, os.Setenv("KUBECONFIG", filepath.Join("testdata", "testkube.conf")))
	ag := Args{}
	ns := ag.GetNamespaceFromConfig()
	assert.Equal(t, "prod", ns)
}
