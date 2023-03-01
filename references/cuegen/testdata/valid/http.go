/*
Copyright 2023 The KubeVela Authors.

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

package test

import (
	"net/http"

	"github.com/kubevela/pkg/cue/cuex/providers"
)

// RequestVars is the vars for http request
// TODO: support timeout & tls
type RequestVars struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Request struct {
		Body    string      `json:"body"`
		Header  http.Header `json:"header"`
		Trailer http.Header `json:"trailer"`
	} `json:"request"`
}

// ResponseVars is the vars for http response
type ResponseVars struct {
	Body       string      `json:"body"`
	Header     http.Header `json:"header"`
	Trailer    http.Header `json:"trailer"`
	StatusCode int         `json:"statusCode"`
}

// DoParams is the params for http request
type DoParams providers.Params[RequestVars]

// DoReturns returned struct for http response
type DoReturns providers.Returns[ResponseVars]
