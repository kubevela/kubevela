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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type typer[T any] interface {
	*T
	Type() string
}

// ParsePolicy parse policy for the given type
func ParsePolicy[T any, P typer[T]](app *v1beta1.Application) (*T, error) {
	spec := new(T)
	for _, policy := range app.Spec.Policies {
		if policy.Type == P(spec).Type() && policy.Properties != nil && policy.Properties.Raw != nil {
			if err := json.Unmarshal(policy.Properties.Raw, spec); err != nil {
				return nil, fmt.Errorf("invalid %s policy %s: %w", policy.Type, policy.Name, err)
			}
			return spec, nil
		}
	}
	return nil, nil
}
