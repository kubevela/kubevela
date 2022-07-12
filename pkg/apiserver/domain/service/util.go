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

	"encoding/json"
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
	// the target policy isn't exist yet, put it in.
	return res[:i], len(c) != i
}

// extractPolicyListAndProperty can extract policy from  string-format properties, and return
// map-format properties in order to further update operation.
func extractPolicyListAndProperty(property string) ([]string, map[string]interface{}, error) {
	content := map[string]interface{}{}
	err := json.Unmarshal([]byte(property), &content)
	if err != nil {
		return nil, nil, err
	}
	policies := content["policies"]
	if policies == nil {
		return nil, content, nil
	}
	list, ok := policies.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("the policies incorrrect")
	}
	if len(list) == 0 {
		return nil, content, nil
	}
	res := []string{}
	for _, i := range list {
		res = append(res, i.(string))
	}
	return res, content, nil
}
