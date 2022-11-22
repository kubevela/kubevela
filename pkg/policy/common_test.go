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
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestParseGarbageCollectPolicy(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{
		Policies: []v1beta1.AppPolicy{{Type: "example"}},
	}}
	spec, err := ParsePolicy[v1alpha1.GarbageCollectPolicySpec](app)
	r.NoError(err)
	r.Nil(spec)
	app.Spec.Policies = append(app.Spec.Policies, v1beta1.AppPolicy{
		Type:       "garbage-collect",
		Properties: &runtime.RawExtension{Raw: []byte("bad value")},
	})
	_, err = ParsePolicy[v1alpha1.GarbageCollectPolicySpec](app)
	r.Error(err)
	policySpec := &v1alpha1.GarbageCollectPolicySpec{
		KeepLegacyResource: false,
		Rules: []v1alpha1.GarbageCollectPolicyRule{{
			Selector: v1alpha1.ResourcePolicyRuleSelector{TraitTypes: []string{"a"}},
			Strategy: v1alpha1.GarbageCollectStrategyOnAppUpdate,
		}, {
			Selector: v1alpha1.ResourcePolicyRuleSelector{TraitTypes: []string{"b"}},
			Strategy: v1alpha1.GarbageCollectStrategyNever,
		}},
	}
	bs, err := json.Marshal(policySpec)
	r.NoError(err)
	app.Spec.Policies[1].Properties.Raw = bs
	spec, err = ParsePolicy[v1alpha1.GarbageCollectPolicySpec](app)
	r.NoError(err)
	r.Equal(policySpec, spec)
}

func TestParseApplyOncePolicy(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{
		Policies: []v1beta1.AppPolicy{{Type: "example"}},
	}}
	spec, err := ParsePolicy[v1alpha1.ApplyOncePolicySpec](app)
	r.NoError(err)
	r.Nil(spec)
	app.Spec.Policies = append(app.Spec.Policies, v1beta1.AppPolicy{
		Type:       "apply-once",
		Properties: &runtime.RawExtension{Raw: []byte("bad value")},
	})
	_, err = ParsePolicy[v1alpha1.ApplyOncePolicySpec](app)
	r.Error(err)
	policySpec := &v1alpha1.ApplyOncePolicySpec{Enable: true}
	bs, err := json.Marshal(policySpec)
	r.NoError(err)
	app.Spec.Policies[1].Properties.Raw = bs
	spec, err = ParsePolicy[v1alpha1.ApplyOncePolicySpec](app)
	r.NoError(err)
	r.Equal(policySpec, spec)
}

func TestParseSharedResourcePolicy(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{
		Policies: []v1beta1.AppPolicy{{Type: "example"}},
	}}
	spec, err := ParsePolicy[v1alpha1.SharedResourcePolicySpec](app)
	r.NoError(err)
	r.Nil(spec)
	app.Spec.Policies = append(app.Spec.Policies, v1beta1.AppPolicy{
		Type:       "shared-resource",
		Properties: &runtime.RawExtension{Raw: []byte("bad value")},
	})
	_, err = ParsePolicy[v1alpha1.SharedResourcePolicySpec](app)
	r.Error(err)
	policySpec := &v1alpha1.SharedResourcePolicySpec{
		Rules: []v1alpha1.SharedResourcePolicyRule{{
			Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceNames: []string{"example"}},
		}}}
	bs, err := json.Marshal(policySpec)
	r.NoError(err)
	app.Spec.Policies[1].Properties.Raw = bs
	spec, err = ParsePolicy[v1alpha1.SharedResourcePolicySpec](app)
	r.NoError(err)
	r.Equal(policySpec, spec)
}

func TestParsePolicy(t *testing.T) {
	r := require.New(t)
	// Test skipping empty policy
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{
		Policies: []v1beta1.AppPolicy{{Type: v1alpha1.GarbageCollectPolicyType, Name: "s", Properties: nil}},
	}}
	exists, err := ParsePolicy[v1alpha1.GarbageCollectPolicySpec](app)
	r.Nil(exists)
	r.NoError(err)
}
