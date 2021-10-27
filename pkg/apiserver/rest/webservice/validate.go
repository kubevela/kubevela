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
)

var validate = validator.New()

var nameRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

const (
	minPageSize = 5
	maxPageSize = 100
)

func init() {
	if err := validate.RegisterValidation("checkname", ValidateName); err != nil {
		panic(err)
	}
}

// ValidateName custom check name field
func ValidateName(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if len(value) > 32 || len(value) < 2 {
		return false
	}
	return nameRegexp.MatchString(value)
}
