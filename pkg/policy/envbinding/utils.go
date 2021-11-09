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
	"encoding/json"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// GetEnvBindingPolicy extract env-binding policy with given policy name, if policy name is empty, the first env-binding policy will be used
func GetEnvBindingPolicy(app *v1beta1.Application, policyName string) (*v1alpha1.EnvBindingSpec, error) {
	for _, policy := range app.Spec.Policies {
		if (policy.Name == policyName || policyName == "") && policy.Type == v1alpha1.EnvBindingPolicyType {
			envBindingSpec := &v1alpha1.EnvBindingSpec{}
			err := json.Unmarshal(policy.Properties.Raw, envBindingSpec)
			return envBindingSpec, err
		}
	}
	return nil, nil
}

// GetEnvBindingPolicyStatus extract env-binding policy status with given policy name, if policy name is empty, the first env-binding policy will be used
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
