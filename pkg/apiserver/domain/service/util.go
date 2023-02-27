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
	// In mongodb, the storage type of policies is array,
	// but the array type cannot be converted to interface type,
	// so we can get the policies by reflection.
	var res []string
	kind := reflect.TypeOf(policies).Kind()
	switch kind {
	case reflect.Slice, reflect.Array:
		s := reflect.ValueOf(policies)
		for i := 0; i < s.Len(); i++ {
			res = append(res, s.Index(i).Interface().(string))
		}
	default:
		// other type not supported
	}
	return res, property, nil
}
