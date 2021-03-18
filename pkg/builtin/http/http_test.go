package http

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"

	"github.com/oam-dev/kubevela/pkg/builtin/registry"
)

const Req = `
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

func TestHTTPCmdRun(t *testing.T) {
	s := NewMock()
	defer s.Close()

	r := cue.Runtime{}
	reqInst, err := r.Compile("", Req)
	if err != nil {
		t.Fatal(err)
	}

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
