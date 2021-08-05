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
	"io/ioutil"
	"net/http"
	"testing"

	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestHttpDo(t *testing.T) {
    runMockServer(t)
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

func runMockServer(t *testing.T)  {
	http.HandleFunc("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("hello"))
	})
	http.HandleFunc("/echo", func(w http.ResponseWriter, req *http.Request) {
		bt, err := ioutil.ReadAll(req.Body)
		assert.NilError(t, err)
		w.Write(bt)
	})
	go http.ListenAndServe(":1229",nil)
}
