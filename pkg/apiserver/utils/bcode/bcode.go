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
	"errors"
	"fmt"

	"github.com/emicklei/go-restful/v3"
	"github.com/go-playground/validator/v10"

	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// Business Code of VelaUX contains 5 digits, the first 3 digits should be reversed and indicates the category of concept
// the last two digits indicates the error number
// For example, business code 11001 should split to 110 and 01, it means the code belongs to the 011 category env, and it's the 01 number error.

// ErrServer an unexpected mistake.
var ErrServer = NewBcode(500, 500, "The service has lapsed.")

// ErrForbidden check user perms failure
var ErrForbidden = NewBcode(403, 403, "403 Forbidden")

// ErrUnauthorized check user auth failure
var ErrUnauthorized = NewBcode(401, 401, "401 Unauthorized")

// Bcode business error code
type Bcode struct {
	HTTPCode     int32 `json:"-"`
	BusinessCode int32
	Message      string
}

func (b *Bcode) Error() string {
	return fmt.Sprintf("HTTPCode:%d BusinessCode:%d Message:%s", b.HTTPCode, b.BusinessCode, b.Message)
}

// SetMessage set new message and return a new bcode instance
func (b *Bcode) SetMessage(message string) *Bcode {
	return &Bcode{
		HTTPCode:     b.HTTPCode,
		BusinessCode: b.BusinessCode,
		Message:      message,
	}
}

var bcodeMap map[int32]*Bcode

// NewBcode new business code
func NewBcode(httpCode, businessCode int32, message string) *Bcode {
	if bcodeMap == nil {
		bcodeMap = make(map[int32]*Bcode)
	}
	if _, exit := bcodeMap[businessCode]; exit {
		panic("bcode business code is exist")
	}
	bcode := &Bcode{HTTPCode: httpCode, BusinessCode: businessCode, Message: message}
	bcodeMap[businessCode] = bcode
	return bcode
}

// ReturnError Unified handling of all types of errors, generating a standard return structure.
func ReturnError(req *restful.Request, res *restful.Response, err error) {
	var bcode *Bcode
	if errors.As(err, &bcode) {
		if err := res.WriteHeaderAndEntity(int(bcode.HTTPCode), err); err != nil {
			log.Logger.Error("write entity failure %s", err.Error())
		}
		return
	}

	if errors.Is(err, datastore.ErrRecordNotExist) {
		if err := res.WriteHeaderAndEntity(int(404), err); err != nil {
			log.Logger.Error("write entity failure %s", err.Error())
		}
		return
	}
	var restfulerr restful.ServiceError
	if errors.As(err, &restfulerr) {
		if err := res.WriteHeaderAndEntity(restfulerr.Code, Bcode{HTTPCode: int32(restfulerr.Code), BusinessCode: int32(restfulerr.Code), Message: restfulerr.Message}); err != nil {
			log.Logger.Error("write entity failure %s", err.Error())
		}
		return
	}

	var validErr validator.ValidationErrors
	if errors.As(err, &validErr) {
		if err := res.WriteHeaderAndEntity(400, Bcode{HTTPCode: 400, BusinessCode: 400, Message: err.Error()}); err != nil {
			log.Logger.Error("write entity failure %s", err.Error())
		}
		return
	}

	log.Logger.Errorf("Business exceptions, error message: %s, path:%s method:%s", err.Error(), utils.Sanitize(req.Request.URL.String()), req.Request.Method)
	if err := res.WriteHeaderAndEntity(500, Bcode{HTTPCode: 500, BusinessCode: 500, Message: err.Error()}); err != nil {
		log.Logger.Error("write entity failure %s", err.Error())
	}
}
