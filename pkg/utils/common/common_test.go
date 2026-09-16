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
	"encoding/pem"
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

	"cuelang.org/go/cue/cuecontext"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"cuelang.org/go/cue/load"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

var ResponseString = "Hello HTTP Get."

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

func TestHTTPGetResponse_BearerToken(t *testing.T) {
	var gotAuth string
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer testServer.Close()

	opts := &HTTPOption{BearerToken: "abc.def.ghi"}
	resp, err := HTTPGetResponse(context.Background(), testServer.URL, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "Bearer abc.def.ghi" {
		t.Fatalf("expected Authorization=%q, got %q", "Bearer abc.def.ghi", gotAuth)
	}
}

func TestHTTPGetResponse_BearerAndBasicMutuallyExclusive(t *testing.T) {
	opts := &HTTPOption{Username: "u", Password: "p", BearerToken: "t"}
	_, err := HTTPGetResponse(context.Background(), "https://example.com", opts)
	if err == nil {
		t.Fatal("expected error when both basic and bearer are set, got nil")
	}
	if !strings.Contains(err.Error(), "RFC 6750") {
		t.Fatalf("expected error to cite RFC 6750, got %v", err)
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

// TestSameOrigin pins the origin comparison the addon registry uses to decide
// whether an index-supplied chart URL may carry the registry's credentials.
// The default port has to normalize, or a repository configured without one
// loses its credentials for an index entry that spells the same origin with
// it; the scheme has to match, or credentials configured for https reach an
// http URL in the clear.
func TestSameOrigin(t *testing.T) {
	cases := map[string]struct {
		a, b string
		want bool
	}{
		"identical":                          {"https://example.com/charts", "https://example.com", true},
		"implicit and explicit 443":          {"https://example.com", "https://example.com:443/x", true},
		"implicit and explicit 80":           {"http://example.com/x", "http://example.com:80", true},
		"host case is insignificant":         {"https://Example.COM", "https://example.com", true},
		"different port":                     {"https://example.com:8443", "https://example.com", false},
		"different host":                     {"https://example.com", "https://mirror.example.com", false},
		"scheme downgrade is a mismatch":     {"https://example.com", "http://example.com", false},
		"unparseable input is never a match": {"https://[::1", "https://[::1", false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, SameOrigin(tc.a, tc.b))
		})
	}
}

// TestHTTPGetRedirectHandlingWithClientCert covers the redirect guard.
//
// net/http drops the Authorization header when a redirect crosses to another
// origin, but a TLS client certificate lives in the transport and is presented
// to whatever host the chain lands on. A repository that redirects its chart
// URLs off-origin would therefore authenticate the caller to a third party,
// which the same-host check on the URL itself cannot prevent because the
// crossing only happens after the request is sent.
func TestHTTPGetRedirectHandlingWithClientCert(t *testing.T) {
	clientCert, err := os.ReadFile("./testdata/server.crt")
	assert.NoError(t, err)
	clientKey, err := os.ReadFile("./testdata/server.key")
	assert.NoError(t, err)

	var followed bool
	elsewhere := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		followed = true
		_, _ = rw.Write([]byte("landed on the redirect target"))
	}))
	defer elsewhere.Close()

	redirector := httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, elsewhere.URL+"/index.yaml", http.StatusFound)
	}))
	defer redirector.Close()

	// Trust both servers, so that a refusal is this code's decision rather than
	// a TLS handshake that would have failed anyway.
	caFile := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: redirector.Certificate().Raw})) +
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: elsewhere.Certificate().Raw}))

	t.Run("a cross-origin redirect is refused while a client certificate is set", func(t *testing.T) {
		followed = false
		_, err := HTTPGetWithOption(context.Background(), redirector.URL+"/index.yaml", &HTTPOption{
			CaFile:   caFile,
			CertFile: string(clientCert),
			KeyFile:  string(clientKey),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "refusing to follow the redirect")
		assert.False(t, followed, "the redirect target must never be contacted")
	})

	t.Run("without a client certificate the redirect is still followed", func(t *testing.T) {
		followed = false
		body, err := HTTPGetWithOption(context.Background(), redirector.URL+"/index.yaml", &HTTPOption{CaFile: caFile})
		assert.NoError(t, err)
		assert.Equal(t, "landed on the redirect target", string(body))
		assert.True(t, followed, "the guard must be scoped to requests that present a client certificate")
	})
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
			_, err := GetCUEParameterValue(tc.cueStr)
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
			_, err := GetCUEParameterValue(tc.cueStr)
			if diff := cmp.Diff(tc.want.errMsg, err.Error(), test.EquateConditions()); diff != "" {
				t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

		})
	}
}

func TestGetCUExParameterValue(t *testing.T) {
	ctx := context.Background()
	type want struct {
		err error
	}

	var validCueStr = `
parameter: {
	min: int
}
`

	var cueStrNotContainParameter = `
output: {
	min: int
}
`

	cases := map[string]struct {
		reason string
		cueStr string
		want   want
	}{
		"ValidCUEString": {
			reason: "cue string is valid",
			cueStr: validCueStr,
			want: want{
				err: nil,
			},
		},
		"CUEStringNotContainParameter": {
			reason: "cue string doesn't contain Parameter",
			cueStr: cueStrNotContainParameter,
			want: want{
				err: fmt.Errorf("parameter not exist"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			val, err := GetCUExParameterValue(ctx, tc.cueStr)
			if tc.want.err != nil {
				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nGetCUExParameterValue(...): -want error, +got error:\n%s", tc.reason, diff)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, val.Exists(), "parameter value should exist")
			}
		})
	}
}

func TestGetCUExParameterValueWithImports(t *testing.T) {
	ctx := context.Background()

	// Setup a test package similar to webhook/utils test
	packageObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cue.oam.dev/v1alpha1",
			"kind":       "Package",
			"metadata": map[string]interface{}{
				"name":      "test-package",
				"namespace": "vela-system",
			},
			"spec": map[string]interface{}{
				"path": "test/builtin",
				"templates": map[string]interface{}{
					"test/builtin": strings.TrimSpace(`
						package builtin
						#Config: {
							name: string
							value: string
						}
					`),
				},
			},
		},
	}

	dcl := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), packageObj)
	singleton.DynamicClient.Set(dcl)
	cuex.DefaultCompiler.Reload()

	defer cuex.DefaultCompiler.Reload()
	defer singleton.ReloadClients()

	var cueStrWithImport = `
import "test/builtin"

parameter: {
	name: string
	config: builtin.#Config
}
`

	val, err := GetCUExParameterValue(ctx, cueStrWithImport)
	assert.NoError(t, err, "should compile CUE with cuex import")
	assert.True(t, val.Exists(), "parameter value should exist")

	// Verify we can access the parameter fields
	iter, err := val.Fields()
	assert.NoError(t, err)

	fieldCount := 0
	for iter.Next() {
		fieldCount++
	}
	assert.Equal(t, 2, fieldCount, "should have 2 parameter fields: name and config")
}

func TestGetCUExParameterValueWithCustomCompiler(t *testing.T) {
	ctx := context.Background()

	var validCueStr = `
parameter: {
	name: string
}
`
	// Pass the default compiler explicitly - exercises the custom compiler path
	compiler := cuex.DefaultCompiler.Get()
	val, err := GetCUExParameterValue(ctx, validCueStr, compiler)
	assert.NoError(t, err, "should work with explicit compiler")
	assert.True(t, val.Exists(), "parameter value should exist")

	// Passing nil compiler falls back to default
	val, err = GetCUExParameterValue(ctx, validCueStr, nil)
	assert.NoError(t, err, "nil compiler should fall back to default")
	assert.True(t, val.Exists(), "parameter value should exist with nil compiler")
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
			val := cuecontext.New().BuildInstance(instances[0])
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

func TestGenOpenAPIWithCueX(t *testing.T) {
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
			val := cuecontext.New().BuildInstance(instances[0])
			assert.NoError(t, val.Err())
			got, err := GenOpenAPIWithCueX(val)
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
	cuectx := cuecontext.New()
	val := cuectx.CompileString(s)
	assert.NoError(t, val.Err())
	_, err := RefineParameterValue(val)
	assert.NoError(t, err)
	// test #parameter not exist but parameter exists
	s = `parameter: {
	x?: string
	if x != "" {
	y: string
	}
}`
	val = cuectx.CompileString(s)
	assert.NoError(t, val.Err())
	assert.NoError(t, err)
	_, err = RefineParameterValue(val)
	assert.NoError(t, err)
	// test #parameter as int
	s = `parameter: #parameter
#parameter: int`
	val = cuectx.CompileString(s)
	assert.NoError(t, err)
	assert.NoError(t, val.Err())
	_, err = RefineParameterValue(val)
	assert.NoError(t, err)
}

func TestFillParameterDefinitionFieldIfNotExist(t *testing.T) {
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
	val := cuecontext.New().CompileString(s)
	assert.NoError(t, val.Err())
	filledVal := FillParameterDefinitionFieldIfNotExist(val)
	assert.NoError(t, filledVal.Err())

	// test #parameter not exist but parameter exists
	s = `parameter: {
		x?: string
		if x != "" {
		y: string
		}
	}`
	val = cuecontext.New().CompileString(s)
	assert.NoError(t, val.Err())
	filledVal = FillParameterDefinitionFieldIfNotExist(val)
	assert.NoError(t, filledVal.Err())

	// test #parameter as int
	s = `parameter: #parameter
	#parameter: int`
	val = cuecontext.New().CompileString(s)
	assert.NoError(t, val.Err())
	filledVal = FillParameterDefinitionFieldIfNotExist(val)
	assert.NoError(t, filledVal.Err())
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
