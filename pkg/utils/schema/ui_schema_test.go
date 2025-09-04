/*
Copyright 2022 The KubeVela Authors.

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

package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultUIType(t *testing.T) {
	testCases := []struct {
		name        string
		apiType     string
		haveOptions bool
		subType     string
		haveSub     bool
		expected    string
	}{
		{
			name:        "string with options",
			apiType:     "string",
			haveOptions: true,
			expected:    "Select",
		},
		{
			name:        "string without options",
			apiType:     "string",
			haveOptions: false,
			expected:    "Input",
		},
		{
			name:     "number",
			apiType:  "number",
			expected: "Number",
		},
		{
			name:     "integer",
			apiType:  "integer",
			expected: "Number",
		},
		{
			name:     "boolean",
			apiType:  "boolean",
			expected: "Switch",
		},
		{
			name:     "array of strings",
			apiType:  "array",
			subType:  "string",
			expected: "Strings",
		},
		{
			name:     "array of numbers",
			apiType:  "array",
			subType:  "number",
			expected: "Numbers",
		},
		{
			name:     "array of integers",
			apiType:  "array",
			subType:  "integer",
			expected: "Numbers",
		},
		{
			name:     "array of structs",
			apiType:  "array",
			subType:  "object",
			expected: "Structs",
		},
		{
			name:     "object with sub-parameters",
			apiType:  "object",
			haveSub:  true,
			expected: "Group",
		},
		{
			name:     "object without sub-parameters",
			apiType:  "object",
			haveSub:  false,
			expected: "KV",
		},
		{
			name:     "unknown type",
			apiType:  "unknown",
			expected: "Input",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uiType := GetDefaultUIType(tc.apiType, tc.haveOptions, tc.subType, tc.haveSub)
			assert.Equal(t, tc.expected, uiType)
		})
	}
}

func TestUISchema_Validate(t *testing.T) {
	tests := []struct {
		name        string
		u           UISchema
		wantErr     bool
		errContains string
	}{
		{
			name: "valid schema",
			u: UISchema{
				{
					Conditions: []Condition{
						{
							JSONKey: "key",
							Op:      "==",
							Action:  "enable",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid condition with empty jsonkey",
			u: UISchema{
				{
					Conditions: []Condition{
						{
							JSONKey: "",
						},
					},
				},
			},
			wantErr:     true,
			errContains: "the json key of the condition can not be empty",
		},
		{
			name: "invalid condition with bad action",
			u: UISchema{
				{
					Conditions: []Condition{
						{
							JSONKey: "key",
							Action:  "badAction",
						},
					},
				},
			},
			wantErr:     true,
			errContains: "the action of the condition only supports enable, disable or leave it empty",
		},
		{
			name: "invalid condition with bad op",
			u: UISchema{
				{
					Conditions: []Condition{
						{
							JSONKey: "key",
							Op:      "badOp",
						},
					},
				},
			},
			wantErr:     true,
			errContains: "the op of the condition must be `==` 、`!=` and `in`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.u.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCondition_Validate(t *testing.T) {
	tests := []struct {
		name        string
		c           Condition
		wantErr     bool
		errContains string
	}{
		{
			name: "valid case with defaults",
			c: Condition{
				JSONKey: "myKey",
				Value:   "myValue",
			},
			wantErr: false,
		},
		{
			name: "valid case with all fields",
			c: Condition{
				JSONKey: "myKey",
				Op:      "==",
				Value:   "myValue",
				Action:  "enable",
			},
			wantErr: false,
		},
		{
			name: "valid op !=",
			c: Condition{
				JSONKey: "myKey",
				Op:      "!=",
			},
			wantErr: false,
		},
		{
			name: "valid op in",
			c: Condition{
				JSONKey: "myKey",
				Op:      "in",
			},
			wantErr: false,
		},
		{
			name: "valid action disable",
			c: Condition{
				JSONKey: "myKey",
				Action:  "disable",
			},
			wantErr: false,
		},
		{
			name: "invalid empty jsonkey",
			c: Condition{
				JSONKey: "",
			},
			wantErr:     true,
			errContains: "the json key of the condition can not be empty",
		},
		{
			name: "invalid action",
			c: Condition{
				JSONKey: "myKey",
				Action:  "invalidAction",
			},
			wantErr:     true,
			errContains: "the action of the condition only supports enable, disable or leave it empty",
		},
		{
			name: "invalid op",
			c: Condition{
				JSONKey: "myKey",
				Op:      ">",
			},
			wantErr:     true,
			errContains: "the op of the condition must be `==` 、`!=` and `in`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}