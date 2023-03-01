package test

import (
	"github.com/kubevela/pkg/cue/cuex/providers"
	"net/http"
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
