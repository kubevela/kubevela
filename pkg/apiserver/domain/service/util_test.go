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
		res struct {
			res        []string
			needUpdate bool
		}
	}{
		{
			name: "containOne",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy2"}, p: "policy2"},
			res: struct {
				res        []string
				needUpdate bool
			}{
				res:        []string{"policy1"},
				needUpdate: true,
			},
		},
		{
			name: "containMulti",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy2", "policy3", "policy2"}, p: "policy2"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{"policy1", "policy3"}, needUpdate: true},
		},
		{
			name: "not-contain",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy3"}, p: "policy2"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{"policy1", "policy3"}, needUpdate: false},
		},
		{
			name: "first-element",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1", "policy3"}, p: "policy1"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{"policy3"}, needUpdate: true},
		},
		{
			name: "only-one",
			input: struct {
				list []string
				p    string
			}{list: []string{"policy1"}, p: "policy1"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{}, needUpdate: true},
		},
	}
	for _, s := range testCases {
		t.Run(s.name, func(t *testing.T) {
			res, needUpdate := guaranteePolicyNotExist(s.input.list, s.input.p)
			assert.DeepEqual(t, res, s.res.res)
			assert.Equal(t, needUpdate, s.res.needUpdate)
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
		res struct {
			res        []string
			needUpdate bool
		}
	}{
		{
			name: "not-contain",
			input: struct {
				list   []string
				policy string
			}{list: []string{"policy1", "policy2"}, policy: "policy3"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{"policy1", "policy2", "policy3"}, needUpdate: true},
		},
		{
			name: "contain-already",
			input: struct {
				list   []string
				policy string
			}{list: []string{"policy1", "policy2"}, policy: "policy2"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{"policy1", "policy2"}, needUpdate: false},
		},
		{
			name: "empty",
			input: struct {
				list   []string
				policy string
			}{list: []string{}, policy: "policy2"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{"policy2"}, needUpdate: true},
		},
		{
			name: "nil slice",
			input: struct {
				list   []string
				policy string
			}{list: nil, policy: "policy2"},
			res: struct {
				res        []string
				needUpdate bool
			}{res: []string{"policy2"}, needUpdate: true},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			res, needUpdate := guaranteePolicyExist(testCase.input.list, testCase.input.policy)
			assert.DeepEqual(t, res, testCase.res.res)
			assert.Equal(t, needUpdate, testCase.res.needUpdate)
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
