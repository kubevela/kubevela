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

package policy

import (
	"encoding/json"
	"fmt"

	"github.com/kubevela/pkg/util/slices"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type typer[T any] interface {
	*T
	Type() string
}

// ParsePolicy parse policy for the given type
func ParsePolicy[T any, P typer[T]](app *v1beta1.Application) (*T, error) {
	base := new(T)
	policies := slices.Filter(app.Spec.Policies, func(policy v1beta1.AppPolicy) bool {
		return policy.Type == P(base).Type() && policy.Properties != nil && policy.Properties.Raw != nil
	})
	if len(policies) == 0 {
		return nil, nil
	}
	props := slices.Map(policies, func(policy v1beta1.AppPolicy) *runtime.RawExtension { return policy.Properties })
	arr := make([]map[string]interface{}, len(props))
	if err := convertType(props, &arr); err != nil {
		return nil, err
	}
	obj := slices.Reduce(arr[1:], mergePolicies, arr[0])
	if err := convertType(obj, base); err != nil {
		return nil, err
	}
	return base, nil
}

// mergePolicies merge two policy spec in place
// 1. for array, concat them
// 2. for bool, return if any of them is true
// 3. otherwise, return base value
func mergePolicies(base, patch map[string]interface{}) map[string]interface{} {
	for k, v := range patch {
		old, found := base[k]
		if !found {
			base[k] = v
			continue
		}
		arr1, ok1 := old.([]interface{})
		arr2, ok2 := v.([]interface{})
		if ok1 && ok2 {
			base[k] = append(arr1, arr2...)
			continue
		}
		m1, ok1 := old.(map[string]interface{})
		m2, ok2 := v.(map[string]interface{})
		if ok1 && ok2 {
			base[k] = mergePolicies(m1, m2)
			continue
		}
		if old == false && v == true {
			base[k] = true
		}
	}
	return base
}

func convertType(src, dest interface{}) error {
	bs, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("failed to marshal %T: %w", src, err)
	}
	if err = json.Unmarshal(bs, dest); err != nil {
		return fmt.Errorf("failed to unmarshal %T: %w", dest, err)
	}
	return nil
}
