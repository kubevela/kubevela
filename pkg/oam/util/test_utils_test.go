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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestAlreadyExistMatcher(t *testing.T) {
	type want struct {
		success bool
		err     error
	}

	cases := map[string]struct {
		input interface{}
		want  want
	}{
		"Matches": {
			input: errors.NewAlreadyExists(schema.GroupResource{
				Group:    "g",
				Resource: "r",
			}, "name"),
			want: want{
				success: true,
				err:     nil,
			},
		},
		"Does not match": {
			input: errors.NewNotFound(schema.GroupResource{
				Group:    "g",
				Resource: "r",
			}, "name"),
			want: want{
				success: false,
				err:     nil,
			},
		},
		"Does not match nil": {
			input: nil,
			want: want{
				success: false,
				err:     nil,
			},
		},
	}
	matcher := AlreadyExistMatcher{}
	for name, tc := range cases {
		success, err := matcher.Match(tc.input)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, tc.want.success, success)
		if tc.want.err == nil {
			assert.NoError(t, err)
		} else {
			assert.Error(t, tc.want.err, err)
		}
	}

	// Error messages
	assert.Equal(t, "Expected\n    <string>: myerror\nto be already exist", matcher.FailureMessage("myerror"))
	assert.Equal(t, "Expected\n    <string>: myerror\nnot to be already exist", matcher.NegatedFailureMessage("myerror"))
}

func TestNotFoundMatcher(t *testing.T) {
	type want struct {
		success bool
		err     error
	}

	cases := map[string]struct {
		input interface{}
		want  want
	}{
		"Does not matche": {
			input: errors.NewAlreadyExists(schema.GroupResource{
				Group:    "g",
				Resource: "r",
			}, "name"),
			want: want{
				success: false,
				err:     nil,
			},
		},
		"Matches": {
			input: errors.NewNotFound(schema.GroupResource{
				Group:    "g",
				Resource: "r",
			}, "name"),
			want: want{
				success: true,
				err:     nil,
			},
		},
		"Does not match nil": {
			input: nil,
			want: want{
				success: false,
				err:     nil,
			},
		},
	}

	matcher := NotFoundMatcher{}
	for name, tc := range cases {
		success, err := matcher.Match(tc.input)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, tc.want.success, success)
		if tc.want.err == nil {
			assert.NoError(t, err)
		} else {
			assert.Equal(t, tc.want.err, err)
		}
	}

	// Error messages
	assert.Equal(t, "Expected\n    <string>: myerror\nto be not found", matcher.FailureMessage("myerror"))
	assert.Equal(t, "Expected\n    <string>: myerror\nnot to be not found", matcher.NegatedFailureMessage("myerror"))
}
func TestErrorMatcher(t *testing.T) {
	type input struct {
		expected error
		input    error
	}

	type want struct {
		success               bool
		err                   error
		failureMessage        string
		negatedFailureMessage string
	}

	cases := map[string]struct {
		input input
		want  want
	}{
		"Matches": {
			input: input{
				expected: fmt.Errorf("my error"),
				input:    fmt.Errorf("my error"),
			},
			want: want{
				success:               true,
				err:                   nil,
				failureMessage:        "Expected\n    <string>: my error\nto equal\n    <string>: my error",
				negatedFailureMessage: "Expected\n    <string>: my error\nnot to equal\n    <string>: my error",
			},
		},
		"Matches nil": {
			input: input{
				expected: nil,
				input:    nil,
			},
			want: want{
				success:               true,
				err:                   nil,
				failureMessage:        "Expected\n    <nil>: nil\nto equal\n    <nil>: nil",
				negatedFailureMessage: "Expected\n    <nil>: nil\nnot to equal\n    <nil>: nil",
			},
		},
		"Does not match": {
			input: input{
				expected: fmt.Errorf("my error"),
				input:    fmt.Errorf("my other error"),
			},
			want: want{
				success:               false,
				err:                   nil,
				failureMessage:        "Expected\n    <string>: my other error\nto equal\n    <string>: my error",
				negatedFailureMessage: "Expected\n    <string>: my other error\nnot to equal\n    <string>: my error",
			},
		},
		"Does not match nil": {
			input: input{
				expected: fmt.Errorf("my error"),
				input:    nil,
			},
			want: want{
				success:               false,
				err:                   nil,
				failureMessage:        "Expected\n    <nil>: nil\nto equal\n    <string>: my error",
				negatedFailureMessage: "Expected\n    <nil>: nil\nnot to equal\n    <string>: my error",
			},
		},
	}
	for name, tc := range cases {
		matcher := ErrorMatcher{
			ExpectedError: tc.input.expected,
		}
		success, err := matcher.Match(tc.input.input)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, tc.want.success, success)
		if tc.want.err == nil {
			assert.NoError(t, err)
		} else {
			assert.Equal(t, tc.want.err, err)
		}

		assert.Equal(t, tc.want.failureMessage, matcher.FailureMessage(tc.input.input))
		assert.Equal(t, tc.want.negatedFailureMessage, matcher.NegatedFailureMessage(tc.input.input))
	}
}

func TestCheckAppRevision(t *testing.T) {
	testcases := map[string]struct {
		revs       []v1beta1.ApplicationRevision
		collection []int
		want       bool
		hasError   bool
	}{
		"match": {
			revs: []v1beta1.ApplicationRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v3",
					},
				},
			},
			collection: []int{1, 2, 3},
			want:       true,
			hasError:   false,
		},
		"lengthNotMatch": {
			revs: []v1beta1.ApplicationRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v2",
					},
				},
			},
			collection: []int{1, 2, 3},
			want:       false,
			hasError:   false,
		},
		"notMatch": {
			revs: []v1beta1.ApplicationRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v2",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v4",
					},
				},
			},
			collection: []int{1, 2, 3},
			want:       false,
			hasError:   false,
		},
		"testSort": {
			revs: []v1beta1.ApplicationRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v3",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v2",
					},
				},
			},
			collection: []int{3, 2, 1},
			want:       true,
			hasError:   false,
		},
		"testErrorName": {
			revs: []v1beta1.ApplicationRevision{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-v3",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "app-vs",
					},
				},
			},
			collection: []int{3, 2, 1},
			want:       false,
			hasError:   true,
		},
	}
	for name, testcase := range testcases {
		t.Log(fmt.Sprint("Running test: ", name))
		checkEqual, err := CheckAppRevision(testcase.revs, testcase.collection)
		hasError := err != nil
		assert.Equal(t, checkEqual, testcase.want)
		assert.Equal(t, hasError, testcase.hasError)
	}
}
