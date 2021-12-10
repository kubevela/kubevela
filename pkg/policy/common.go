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

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// parsePolicy parse policy for application
func parsePolicy(app *v1beta1.Application, policyType string, policySpec interface{}) (exists bool, err error) {
	for _, policy := range app.Spec.Policies {
		if policy.Type == policyType && policy.Properties != nil && policy.Properties.Raw != nil {
			if err := json.Unmarshal(policy.Properties.Raw, policySpec); err != nil {
				return true, errors.Wrapf(err, "invalid %s policy: %s", policy.Type, policy.Name)
			}
			return true, nil
		}
	}
	return false, nil
}

// ParseGarbageCollectPolicy parse garbage-collect policy
func ParseGarbageCollectPolicy(app *v1beta1.Application) (*v1alpha1.GarbageCollectPolicySpec, error) {
	spec := &v1alpha1.GarbageCollectPolicySpec{}
	if exists, err := parsePolicy(app, v1alpha1.GarbageCollectPolicyType, spec); exists {
		return spec, err
	}
	return nil, nil
}

// ParseApplyOncePolicy parse apply-once policy
func ParseApplyOncePolicy(app *v1beta1.Application) (*v1alpha1.ApplyOncePolicySpec, error) {
	spec := &v1alpha1.ApplyOncePolicySpec{}
	if exists, err := parsePolicy(app, v1alpha1.ApplyOncePolicyType, spec); exists {
		return spec, err
	}
	return nil, nil
}
