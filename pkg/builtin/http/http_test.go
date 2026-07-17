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
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"

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

func TestHTTPCmdRunWithCustomTimeout(t *testing.T) {
	// Start a slow server that takes 200ms to respond
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on ephemeral port: %v", err)
	}
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	defer ts.Close()

	// Derive the server URL from the listener address
	serverURL := fmt.Sprintf("http://%s/api/v1/slow", l.Addr().String())

	// Without custom timeout (default 3s) — should succeed since server responds in 200ms
	reqDefault := cuecontext.New().CompileString(fmt.Sprintf(`{
		method: "GET"
		url: "%s"
	}`, serverURL))
	runner, _ := newHTTPCmd(cue.Value{})
	got, err := runner.Run(&registry.Meta{Obj: reqDefault.Value()})
	assert.NoError(t, err)
	body := (got.(map[string]interface{}))["body"].(string)
	assert.Equal(t, `{"status":"ok"}`, body)

	// With explicit timeout of 1s — should also succeed
	reqCustom := cuecontext.New().CompileString(fmt.Sprintf(`{
		method: "GET"
		url: "%s"
		request: {
			timeout: "1s"
		}
	}`, serverURL))
	got, err = runner.Run(&registry.Meta{Obj: reqCustom.Value()})
	assert.NoError(t, err)
	body = (got.(map[string]interface{}))["body"].(string)
	assert.Equal(t, `{"status":"ok"}`, body)

	// With a very short timeout of 50ms — should fail
	reqShort := cuecontext.New().CompileString(fmt.Sprintf(`{
		method: "GET"
		url: "%s"
		request: {
			timeout: "50ms"
		}
	}`, serverURL))
	_, err = runner.Run(&registry.Meta{Obj: reqShort.Value()})
	assert.Error(t, err, "expected timeout error with 50ms deadline on a 200ms server")
}

func TestHTTPCmdRunWithInvalidTimeout(t *testing.T) {
	s := NewMock()
	defer s.Close()

	runner, _ := newHTTPCmd(cue.Value{})

	// Invalid timeout value should be silently ignored and use the default 3s
	reqInvalid := cuecontext.New().CompileString(`{
		method: "GET"
		url: "http://127.0.0.1:8090/api/v1/token?val=test-token"
		request: {
			timeout: "not-a-duration"
			header: {
				"Accept-Language": "en,nl"
			}
		}
	}`)
	got, err := runner.Run(&registry.Meta{Obj: reqInvalid.Value()})
	assert.NoError(t, err)
	body := (got.(map[string]interface{}))["body"].(string)
	assert.Equal(t, `{"token":"test-token"}`, body)

	// Zero timeout should be rejected and fall back to default 3s
	reqZero := cuecontext.New().CompileString(`{
		method: "GET"
		url: "http://127.0.0.1:8090/api/v1/token?val=test-zero"
		request: {
			timeout: "0s"
		}
	}`)
	got, err = runner.Run(&registry.Meta{Obj: reqZero.Value()})
	assert.NoError(t, err)
	body = (got.(map[string]interface{}))["body"].(string)
	assert.Equal(t, `{"token":"test-zero"}`, body)

	// Negative timeout should be rejected and fall back to default 3s
	reqNegative := cuecontext.New().CompileString(`{
		method: "GET"
		url: "http://127.0.0.1:8090/api/v1/token?val=test-negative"
		request: {
			timeout: "-5s"
		}
	}`)
	got, err = runner.Run(&registry.Meta{Obj: reqNegative.Value()})
	assert.NoError(t, err)
	body = (got.(map[string]interface{}))["body"].(string)
	assert.Equal(t, `{"token":"test-negative"}`, body)
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
		t.Fatal(err)
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
	cert, err := tls.X509KeyPair([]byte(decodeCert(testdata.MockCerts.ServerCrt)), []byte(decodeCert(testdata.MockCerts.ServerKey)))
	if err != nil {
		panic(err)
	}
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
