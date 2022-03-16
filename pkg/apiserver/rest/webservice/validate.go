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

package webservice

import (
	"regexp"

	"github.com/go-playground/validator/v10"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
)

var validate = validator.New()

var (
	nameRegexp  = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	emailRegexp = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
)

const (
	minPageSize = 5
	maxPageSize = 100
)

func init() {
	if err := validate.RegisterValidation("checkname", ValidateName); err != nil {
		panic(err)
	}
	if err := validate.RegisterValidation("checkalias", ValidateAlias); err != nil {
		panic(err)
	}
	if err := validate.RegisterValidation("checkpayloadtype", ValidatePayloadType); err != nil {
		panic(err)
	}
	if err := validate.RegisterValidation("checkemail", ValidateEmail); err != nil {
		panic(err)
	}
}

// ValidatePayloadType check PayloadType
func ValidatePayloadType(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	for _, v := range usecase.WebhookHandlers {
		if v == value {
			return true
		}
	}
	return false
}

// ValidateName custom check name field
func ValidateName(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if len(value) > 32 || len(value) < 2 {
		return false
	}
	return nameRegexp.MatchString(value)
}

// ValidateAlias custom check alias field
func ValidateAlias(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value != "" && (len(value) > 64 || len(value) < 2) {
		return false
	}
	return true
}

// ValidateEmail custom check email field
func ValidateEmail(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	return emailRegexp.MatchString(value)
}
