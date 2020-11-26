package util

import (
	"encoding/json"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// JSONMarshal returns the JSON encoding
func JSONMarshal(o interface{}) []byte {
	j, _ := json.Marshal(o)
	return j
}

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
	expectedError, expectedOK := matcher.ExpectedError.(error)

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
	expectedError, expectedOK := matcher.ExpectedError.(error)

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
