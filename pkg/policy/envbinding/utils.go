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

package envbinding

import (
	"context"
	"encoding/json"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type contextKey string

const (
	// EnvNameContextKey is the name of env
	// Deprecated
	EnvNameContextKey = contextKey("EnvName")
)

// GetEnvBindingPolicy extract env-binding policy with given policy name, if policy name is empty, the first env-binding policy will be used
// Deprecated
func GetEnvBindingPolicy(app *v1beta1.Application, policyName string) (*v1alpha1.EnvBindingSpec, error) {
	for _, policy := range app.Spec.Policies {
		if (policy.Name == policyName || policyName == "") && policy.Type == v1alpha1.EnvBindingPolicyType && policy.Properties != nil {
			envBindingSpec := &v1alpha1.EnvBindingSpec{}
			err := json.Unmarshal(policy.Properties.Raw, envBindingSpec)
			return envBindingSpec, err
		}
	}
	return nil, nil
}

// GetEnvBindingPolicyStatus extract env-binding policy status with given policy name, if policy name is empty, the first env-binding policy will be used
// Deprecated
func GetEnvBindingPolicyStatus(app *v1beta1.Application, policyName string) (*v1alpha1.EnvBindingStatus, error) {
	for _, policyStatus := range app.Status.PolicyStatus {
		if (policyStatus.Name == policyName || policyName == "") && policyStatus.Type == v1alpha1.EnvBindingPolicyType {
			envBindingStatus := &v1alpha1.EnvBindingStatus{}
			if policyStatus.Status != nil {
				err := json.Unmarshal(policyStatus.Status.Raw, envBindingStatus)
				return envBindingStatus, err
			}
			return nil, nil
		}
	}
	return nil, nil
}

// EnvNameInContext extract env name from context
// Deprecated
func EnvNameInContext(ctx context.Context) string {
	envName := ctx.Value(EnvNameContextKey)
	if envName != nil {
		return envName.(string)
	}
	return ""
}

// ContextWithEnvName wraps context with envName
// Deprecated
func ContextWithEnvName(ctx context.Context, envName string) context.Context {
	return context.WithValue(ctx, EnvNameContextKey, envName)
}
