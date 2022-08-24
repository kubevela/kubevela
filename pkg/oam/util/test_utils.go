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
	"encoding/json"
	"reflect"
	"sort"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// JSONMarshal returns the JSON encoding
func JSONMarshal(o interface{}) []byte {
	j, _ := json.Marshal(o)
	return j
}

var _ types.GomegaMatcher = AlreadyExistMatcher{}

// AlreadyExistMatcher matches the error to be already exist
type AlreadyExistMatcher struct {
}

// Match matches error.
func (matcher AlreadyExistMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return false, nil
	}
	actualError := actual.(error)
	return apierrors.IsAlreadyExists(actualError), nil
}

// FailureMessage builds an error message.
func (matcher AlreadyExistMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be already exist")
}

// NegatedFailureMessage builds an error message.
func (matcher AlreadyExistMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be already exist")
}

var _ types.GomegaMatcher = NotFoundMatcher{}

// NotFoundMatcher matches the error to be not found.
type NotFoundMatcher struct {
}

// Match matches the api error.
func (matcher NotFoundMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return false, nil
	}
	actualError := actual.(error)
	return apierrors.IsNotFound(actualError), nil
}

// FailureMessage builds an error message.
func (matcher NotFoundMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be not found")
}

// NegatedFailureMessage builds an error message.
func (matcher NotFoundMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be not found")
}

// BeEquivalentToError matches the error to take care of nil.
func BeEquivalentToError(expected error) types.GomegaMatcher {
	return &ErrorMatcher{
		ExpectedError: expected,
	}
}

var _ types.GomegaMatcher = ErrorMatcher{}

// ErrorMatcher matches errors.
type ErrorMatcher struct {
	ExpectedError error
}

// Match matches an error.
func (matcher ErrorMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return matcher.ExpectedError == nil, nil
	}
	actualError := actual.(error)
	return actualError.Error() == matcher.ExpectedError.Error(), nil
}

// FailureMessage builds an error message.
func (matcher ErrorMatcher) FailureMessage(actual interface{}) (message string) {
	actualError, actualOK := actual.(error)
	expectedError := matcher.ExpectedError
	expectedOK := expectedError != nil

	if actualOK && expectedOK {
		return format.MessageWithDiff(actualError.Error(), "to equal", expectedError.Error())
	}

	if actualOK && !expectedOK {
		return format.Message(actualError.Error(), "to equal", expectedError.Error())
	}

	if !actualOK && expectedOK {
		return format.Message(actual, "to equal", expectedError.Error())
	}

	return format.Message(actual, "to equal", expectedError)
}

// NegatedFailureMessage builds an error message.
func (matcher ErrorMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	actualError, actualOK := actual.(error)
	expectedError := matcher.ExpectedError
	expectedOK := expectedError != nil

	if actualOK && expectedOK {
		return format.MessageWithDiff(actualError.Error(), "not to equal", expectedError.Error())
	}

	if actualOK && !expectedOK {
		return format.Message(actualError.Error(), "not to equal", expectedError.Error())
	}

	if !actualOK && expectedOK {
		return format.Message(actual, "not to equal", expectedError.Error())
	}

	return format.Message(actual, "not to equal", expectedError)
}

// UnMarshalStringToComponentDefinition parse a string to a componentDefinition object
func UnMarshalStringToComponentDefinition(s string) (*v1beta1.ComponentDefinition, error) {
	obj := &v1beta1.ComponentDefinition{}
	_body, err := yaml.YAMLToJSON([]byte(s))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(_body, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// UnMarshalStringToWorkloadDefinition parse a string to a workloadDefinition object
func UnMarshalStringToWorkloadDefinition(s string) (*v1alpha2.WorkloadDefinition, error) {
	obj := &v1alpha2.WorkloadDefinition{}
	_body, err := yaml.YAMLToJSON([]byte(s))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(_body, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// UnMarshalStringToTraitDefinition parse a string to a traitDefinition object
func UnMarshalStringToTraitDefinition(s string) (*v1beta1.TraitDefinition, error) {
	obj := &v1beta1.TraitDefinition{}
	_body, err := yaml.YAMLToJSON([]byte(s))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(_body, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// UnMarshalStringToPolicyDefinition parse a string to a policyDefinition object
func UnMarshalStringToPolicyDefinition(s string) (*v1beta1.PolicyDefinition, error) {
	obj := &v1beta1.PolicyDefinition{}
	_body, err := yaml.YAMLToJSON([]byte(s))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(_body, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// CheckAppRevision check if appRevision list is right
func CheckAppRevision(revs []v1beta1.ApplicationRevision, collection []int) (bool, error) {
	if len(revs) != len(collection) {
		return false, nil
	}
	var revNums []int
	for _, rev := range revs {
		num, err := ExtractRevisionNum(rev.Name, "-")
		if err != nil {
			return false, err
		}
		revNums = append(revNums, num)
	}
	sort.Ints(revNums)
	sort.Ints(collection)
	if reflect.DeepEqual(revNums, collection) {
		return true, nil
	}
	return false, nil
}
