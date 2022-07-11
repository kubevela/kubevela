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

package service

import (
	"testing"

	"gotest.tools/assert"
)

func TestGuaranteePolicyNotExist(t *testing.T) {
	testCases := []struct {
		name  string
		input struct {
			list []string
			p    string
		}
		res []string
	}{
		{
			name: "containOne",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy2"}, p: "policy2"},
			res: []string{"policy1"},
		},
		{
			name: "containMulti",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy2", "policy3", "policy2"}, p: "policy2"},
			res: []string{"policy1", "policy3"},
		},
		{
			name: "not-contain",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy3"}, p: "policy2"},
			res: []string{"policy1", "policy3"},
		},
		{
			name: "first-element",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy3"}, p: "policy1"},
			res: []string{"policy3"},
		},
		{
			name: "only-one",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1"}, p: "policy1"},
			res: []string{},
		},
	}
	for _, s := range testCases {
		t.Run(s.name, func(t *testing.T) {
			res := guaranteePolicyNotExist(s.input.list, s.input.p)
			assert.DeepEqual(t, res, s.res)
		})
	}
}

func TestGuaranteePolicyExist(t *testing.T) {
	testCases := []struct {
		name  string
		input struct {
			list   []string
			policy string
		}
		res []string
	}{
		{
			name: "not-contain",
			input: struct {
				list   []string
				policy string
			}{list: []string{"policy1", "policy2"}, policy: "policy3"},
			res: []string{"policy1", "policy2", "policy3"},
		},
		{
			name: "contain-already",
			input: struct {
				list   []string
				policy string
			}{list: []string{"policy1", "policy2"}, policy: "policy2"},
			res: []string{"policy1", "policy2"},
		},
		{
			name: "empty",
			input: struct {
				list   []string
				policy string
			}{list: []string{}, policy: "policy2"},
			res: []string{"policy2"},
		},
		{
			name: "nil slice",
			input: struct {
				list   []string
				policy string
			}{list: nil, policy: "policy2"},
			res: []string{"policy2"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			res := guaranteePolicyExist(testCase.input.list, testCase.input.policy)
			assert.DeepEqual(t, res, testCase.res)
		})
	}
}

func TestExtractPolicyListAndProperty(t *testing.T) {
	testCases := []struct {
		input string
		res   struct {
			policies   []string
			properties map[string]interface{}
			noError    bool
		}
	}{
		{
			input: `{"policies":["policy1","policy2"], "components": ["comp1"]}`,
			res: struct {
				policies   []string
				properties map[string]interface{}
				noError    bool
			}{policies: []string{"policy1", "policy2"}, properties: map[string]interface{}{
				"policies":   []interface{}{"policy1", "policy2"},
				"components": []interface{}{"comp1"},
			}, noError: true},
		},
		{
			input: `{"policies":["policy1"], "components": ["comp1"]}`,
			res: struct {
				policies   []string
				properties map[string]interface{}
				noError    bool
			}{policies: []string{"policy1"}, properties: map[string]interface{}{
				"policies":   []interface{}{"policy1"},
				"components": []interface{}{"comp1"},
			}, noError: true},
		},
		{
			input: `{"policies":["policy1", "components": ["comp1"]}`,
			res: struct {
				policies   []string
				properties map[string]interface{}
				noError    bool
			}{noError: false},
		},
	}
	for _, testCase := range testCases {
		policy, properties, err := extractPolicyListAndProperty(testCase.input)
		if testCase.res.noError {
			assert.NilError(t, err)
		} else {
			assert.Equal(t, err != nil, true)
		}
		assert.DeepEqual(t, policy, testCase.res.policies)
		assert.DeepEqual(t, properties, testCase.res.properties)
	}
}
