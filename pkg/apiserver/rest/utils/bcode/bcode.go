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

package bcode

import (
	"fmt"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/go-playground/validator/v10"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
)

// Bcode business error code
type Bcode struct {
	HTTPCode     int32 `json:"-"`
	BusinessCode int32
	Message      string
}

func (b *Bcode) Error() string {
	return fmt.Sprintf("HTTPCode:%d BusinessCode:%d Message:%s", b.HTTPCode, b.BusinessCode, b.Message)
}

// ReturnError Unified handling of all types of errors, generating a standard return structure.
func ReturnError(req *restful.Request, res *restful.Response, err error) {
	if bcode, ok := err.(*Bcode); ok {
		res.WriteEntity(bcode)
		return
	}
	if serviceError, ok := err.(*restful.ServiceError); ok {
		res.WriteEntity(Bcode{HTTPCode: int32(serviceError.Code), BusinessCode: int32(serviceError.Code), Message: serviceError.Message})
		return
	}

	if validation, ok := err.(*validator.ValidationErrors); ok {
		res.WriteEntity(Bcode{HTTPCode: 400, BusinessCode: 400, Message: validation.Error()})
		return
	}
	log.Logger.Errorf("Business exceptions, message %s, path:%s method:%s", err.Error(), req.Request.URL, req.Request.Method)
	res.WriteEntity(Bcode{HTTPCode: 500, BusinessCode: 500, Message: err.Error()})
}
