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

// ErrorList wraps a list of errors, it also fit for an Error interface
type ErrorList []error

// Error implement error interface
func (e ErrorList) Error() string {
	if !e.HasError() {
		// it reports an empty string if error is nil
		return ""
	}
	errMessages := make([]string, len(e))
	for i, err := range e {
		errMessages[i] = err.Error()
	}
	return fmt.Sprintf("Found %d errors. [(%s)]", len(e), strings.Join(errMessages, "), ("))
}

// HasError check if any error exists in list
func (e ErrorList) HasError() bool {
	if e == nil {
		return false
	}
	return len(e) > 0
}
