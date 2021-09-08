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

package errors

import (
	"fmt"
	"strings"
)

type ErrorList struct {
	errors []error
}

func (e ErrorList) Error() string {
	if !e.HasError() {
		return "No error found."
	}
	errMessages := make([]string, len(e.errors))
	for i, err := range e.errors {
		errMessages[i] = err.Error()
	}
	return fmt.Sprintf("Found %d errors. [(%s)]", len(e.errors), strings.Join(errMessages, "), ("))
}

func (e *ErrorList) Append(err error) {
	e.errors = append(e.errors, err)
}

func (e ErrorList) HasError() bool {
	return len(e.errors) > 0
}