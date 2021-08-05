package http

import (
	"io/ioutil"
	"net/http"
	"testing"

	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestHttpDo(t *testing.T) {
	closeCh := runMockServer(t)
	defer func() {
		close(closeCh)
	}()

	baseTemplate := `
		url: string
		request?: close({
			body:    string
			header:  [string]: string
			trailer: [string]: string
		})
		response: close({
			body: string
			header?:  [string]: [...string]
			trailer?: [string]: [...string]
		})
`
	testCases := map[string]struct {
		request      string
		expectedBody string
	}{
		"hello": {
			request: baseTemplate + `
method: "GET"
url: "http://127.0.0.1:1229/hello"`,
			expectedBody: `hello`,
		},

		"echo": {
			request: baseTemplate + `
method: "POST"
url: "http://127.0.0.1:1229/echo"
request:{ 
   body: "I am vela" 
   header: "Content-Type": "text/plain; charset=utf-8"
}`,
			expectedBody: `I am vela`,
		},
		"json": {
			request: `
import ("encoding/json")
foo: {
	name: "foo"
	score: 100
}

method: "POST"
url: "http://127.0.0.1:1229/echo"
request:{ 
   body: json.Marshal(foo)
   header: "Content-Type": "application/json; charset=utf-8"
}` + baseTemplate,
			expectedBody: `{"name":"foo","score":100}`,
		},
	}

	for tName, tCase := range testCases {
		v, err := value.NewValue(tCase.request, nil)
		assert.NilError(t, err, tName)
		prd := &provider{}
		err = prd.Do(nil, v, nil)
		assert.NilError(t, err, tName)
		body, err := v.LookupValue("response", "body")
		assert.NilError(t, err, tName)
		ret, err := body.CueValue().String()
		assert.NilError(t, err, tName)
		assert.Equal(t, ret, tCase.expectedBody, tName)
	}
}

func runMockServer(t *testing.T) chan struct{} {
	http.HandleFunc("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("hello"))
	})
	http.HandleFunc("/echo", func(w http.ResponseWriter, req *http.Request) {
		bt, err := ioutil.ReadAll(req.Body)
		assert.NilError(t, err)
		w.Write(bt)
	})
	srv := &http.Server{Addr: ":1229"}
	close := make(chan struct{})
	go func() {
		err := srv.ListenAndServe()
		assert.NilError(t, err)
	}()
	go func() {
		<-close
		srv.Close()
	}()
	return close
}
