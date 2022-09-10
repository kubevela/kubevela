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

package http

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/bmizerany/assert"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/builtin/http/testdata"
	"github.com/oam-dev/kubevela/pkg/builtin/registry"
)

const (
	Req = `
{
  method: *"GET" | string
  url: "http://127.0.0.1:8090/api/v1/token?val=test-token"
  request: {
    body ?: bytes
    header: {
    "Accept-Language": "en,nl"
    }
    trailer: {
    "Accept-Language": "en,nl"
    User: "foo"
    }
  }
}
`
	ReqWithoutHeader = `
{
  method: *"GET" | string
  url: "http://127.0.0.1:8090/api/v1/token?val=test-token-no-header"
  request: {
    body ?: bytes
    trailer: {
      "Accept-Language": "en,nl"
      User: "foo"
    }
  }
}
`
)

func TestHTTPCmdRun(t *testing.T) {
	s := NewMock()
	defer s.Close()

	reqInst := cuecontext.New().CompileString(Req)

	runner, _ := newHTTPCmd(cue.Value{})
	got, err := runner.Run(&registry.Meta{Obj: reqInst.Value()})
	if err != nil {
		t.Error(err)
	}
	body := (got.(map[string]interface{}))["body"].(string)

	assert.Equal(t, "{\"token\":\"test-token\"}", body)

	reqNoHeaderInst := cuecontext.New().CompileString(ReqWithoutHeader)
	if err != nil {
		t.Fatal(err)
	}

	got, err = runner.Run(&registry.Meta{Obj: reqNoHeaderInst.Value()})
	if err != nil {
		t.Error(err)
	}
	body = (got.(map[string]interface{}))["body"].(string)

	assert.Equal(t, "{\"token\":\"test-token-no-header\"}", body)

}

func TestHTTPSRun(t *testing.T) {
	s := newMockHttpsServer()
	defer s.Close()
	reqInst := cuecontext.New().CompileString(`method: "GET"
url: "https://127.0.0.1:8443/api/v1/token?val=test-token"`)
	reqInst = reqInst.FillPath(value.FieldPath("tls_config", "ca"), decodeCert(testdata.MockCerts.Ca))
	reqInst = reqInst.FillPath(value.FieldPath("tls_config", "client_crt"), decodeCert(testdata.MockCerts.ClientCrt))
	reqInst = reqInst.FillPath(value.FieldPath("tls_config", "client_key"), decodeCert(testdata.MockCerts.ClientKey))

	runner, _ := newHTTPCmd(cue.Value{})
	got, err := runner.Run(&registry.Meta{Obj: reqInst.Value()})
	if err != nil {
		t.Error(err)
	}
	body := (got.(map[string]interface{}))["body"].(string)

	assert.Equal(t, "{\"token\":\"test-token\"}", body)
}

// NewMock mock the http server
func NewMock() *httptest.Server {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			fmt.Printf("Expected 'GET' request, got '%s'", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/token" {
			fmt.Printf("Expected request to '/person', got '%s'", r.URL.EscapedPath())
		}
		r.ParseForm()
		token := r.Form.Get("val")
		tokenBytes, _ := json.Marshal(map[string]interface{}{"token": token})

		w.WriteHeader(http.StatusOK)
		w.Write(tokenBytes)
	}))
	l, _ := net.Listen("tcp", "127.0.0.1:8090")
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	return ts
}

func newMockHttpsServer() *httptest.Server {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			fmt.Printf("Expected 'GET' request, got '%s'", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/token" {
			fmt.Printf("Expected request to '/person', got '%s'", r.URL.EscapedPath())
		}
		r.ParseForm()
		token := r.Form.Get("val")
		tokenBytes, _ := json.Marshal(map[string]interface{}{"token": token})

		w.WriteHeader(http.StatusOK)
		w.Write(tokenBytes)
	}))
	l, _ := net.Listen("tcp", "127.0.0.1:8443")
	ts.Listener.Close()
	ts.Listener = l

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(decodeCert(testdata.MockCerts.Ca)))
	cert, _ := tls.X509KeyPair([]byte(decodeCert(testdata.MockCerts.ServerCrt)), []byte(decodeCert(testdata.MockCerts.ServerKey)))
	ts.TLS = &tls.Config{
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"http/1.1"},
	}
	ts.StartTLS()
	return ts
}

func decodeCert(in string) string {
	out, _ := base64.StdEncoding.DecodeString(in)
	return string(out)
}
