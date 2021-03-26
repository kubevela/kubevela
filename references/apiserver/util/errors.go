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

package util

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Code defines the error code type.
type Code int

// Be careful, below constants must be added to the errorDetails map.
// All code should be defined between StartMarker and EndMarker
const (
	PathNotSupported Code = iota
	InvalidArgument
	UnsupportedMediaType
	StatusInternalServerError
)

type errorDetail struct {
	ID         string
	StatusCode int
	Message    string
}

var errorDetails = map[Code]errorDetail{
	PathNotSupported:          {"PathNotSupported", http.StatusNotFound, "'%s' against '%s' is not supported"},
	InvalidArgument:           {"InvalidArgument", http.StatusBadRequest, "%s"},
	UnsupportedMediaType:      {"UnsupportedMediaType", http.StatusUnsupportedMediaType, "content type should be 'application/json' or 'application/octet-stream'"},
	StatusInternalServerError: {"StatusInternalServerError", http.StatusInternalServerError, "%s"}}

// ID returns the error ID.
func (c Code) ID() string {
	return errorDetails[c].ID
}

// StatusCode returns the http status code.
func (c Code) StatusCode() int {
	return errorDetails[c].StatusCode
}

// Message returns the detailed error message.
func (c Code) Message() string {
	return errorDetails[c].Message
}

// ConstructError returns a new OpError.
func ConstructError(ec Code, a ...interface{}) error {
	msg := ""
	// the number of keys should be equal to the number of placeholders defined in ErrorCode.Message.
	c := strings.Count(ec.Message(), "%")
	switch {
	case a == nil && c > 0 ||
		a != nil && (c != len(a) || a[0] == nil):
		{
			ctrl.Log.Error(fmt.Errorf("args '%v' do not match placeholders in the msg '%s'", a, ec.Message()),
				"invalid error message argument")
		}
	case len(a) == 0 || a[0] == nil:
		{
			msg = ec.Message()
		}
	default:
		msg = fmt.Sprintf(ec.Message(), a...)
	}
	return errors.New(msg)
}

// SetErrorAndAbort is used to abort the rest of the handlers, mostly called in middleware
func SetErrorAndAbort(c *gin.Context, code Code, msg ...interface{}) {
	// Calling abort so no handlers and middlewares will be executed.
	c.AbortWithStatusJSON(code.StatusCode(), gin.H{"error": ConstructError(code, msg...).Error()})

}

// HandleError will handle error
func HandleError(c *gin.Context, code Code, msg ...interface{}) {
	err := ConstructError(code, msg...)
	AssembleResponse(c, nil, err)
}
