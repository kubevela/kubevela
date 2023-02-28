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
	"fmt"
	"reflect"
)

// guaranteePolicyExist check the slice whether contain the target policy, if not put it in.
// and tell invoker whether should update the policy
func guaranteePolicyExist(c []string, policy string) ([]string, bool) {
	for _, p := range c {
		if policy == p {
			return c, false
		}
	}
	return append(c, policy), true
}

// guaranteePolicyNotExist check the slice whether caontain the target policy, if yes delete
// and tell invoker whether should update the policy
func guaranteePolicyNotExist(c []string, policy string) ([]string, bool) {
	res := make([]string, len(c))
	i := 0
	for _, p := range c {
		if p != policy {
			res[i] = p
			i++
		}
	}
	// if len(c) != i, that's mean target policy exist in the list, this function has delete it from returned result,
	// and outer caller should update with it.
	return res[:i], len(c) != i
}

// extractPolicyListAndProperty can extract policy from  string-format properties, and return
// map-format properties in order to further update operation.
func extractPolicyListAndProperty(property map[string]interface{}) ([]string, map[string]interface{}, error) {
	if len(property) == 0 {
		return nil, nil, nil
	}
	policies := property["policies"]
	if policies == nil {
		return nil, property, nil
	}
	list, err := InterfaceSlice(policies)
	if err != nil {
		return nil, nil, fmt.Errorf("the policies incorrect")
	}
	if len(list) == 0 {
		return nil, property, nil
	}
	var res []string
	for _, i := range list {
		res = append(res, i.(string))
	}
	return res, property, nil
}

// InterfaceSlice interface to []interface{}
func InterfaceSlice(slice interface{}) ([]interface{}, error) {
	if arr, ok := slice.([]interface{}); ok {
		return arr, nil
	}
	s := reflect.ValueOf(slice)
	if s.Kind() != reflect.Slice {
		return nil, fmt.Errorf("InterfaceSlice() given a non-slice type")
	}
	// Keep the distinction between nil and empty slice input
	if s.IsNil() {
		return nil, nil
	}
	ret := make([]interface{}, s.Len())
	for i := 0; i < s.Len(); i++ {
		ret[i] = s.Index(i).Interface()
	}
	return ret, nil
}
