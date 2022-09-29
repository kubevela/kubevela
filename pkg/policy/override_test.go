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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestParseOverridePolicyRelatedDefinitions(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(&v1beta1.ComponentDefinition{
		ObjectMeta: v1.ObjectMeta{Name: "comp", Namespace: oam.SystemDefinitionNamespace},
	}, &v1beta1.TraitDefinition{
		ObjectMeta: v1.ObjectMeta{Name: "trait", Namespace: "test"},
	}).Build()
	r := require.New(t)
	app := &v1beta1.Application{}
	app.SetNamespace("test")
	testCases := map[string]struct {
		Policy        v1beta1.AppPolicy
		ComponentDefs []*v1beta1.ComponentDefinition
		TraitDefs     []*v1beta1.TraitDefinition
		Error         string
	}{
		"normal": {
			Policy:        v1beta1.AppPolicy{Properties: &runtime.RawExtension{Raw: []byte(`{"components":[{"type":"comp","traits":[{"type":"trait"}]}]}`)}},
			ComponentDefs: []*v1beta1.ComponentDefinition{{ObjectMeta: v1.ObjectMeta{Name: "comp", Namespace: oam.SystemDefinitionNamespace}}},
			TraitDefs:     []*v1beta1.TraitDefinition{{ObjectMeta: v1.ObjectMeta{Name: "trait", Namespace: "test"}}},
		},
		"invalid-override-policy": {
			Policy: v1beta1.AppPolicy{Properties: &runtime.RawExtension{Raw: []byte(`{bad value}`)}},
			Error:  "invalid override policy spec",
		},
		"comp-def-not-found": {
			Policy: v1beta1.AppPolicy{Properties: &runtime.RawExtension{Raw: []byte(`{"components":[{"type":"comp-404","traits":[{"type":"trait"}]}]}`)}},
			Error:  "failed to get component definition",
		},
		"trait-def-not-found": {
			Policy: v1beta1.AppPolicy{Properties: &runtime.RawExtension{Raw: []byte(`{"components":[{"type":"comp","traits":[{"type":"trait-404"}]}]}`)}},
			Error:  "failed to get trait definition",
		},
		"empty-policy": {
			Policy:        v1beta1.AppPolicy{Properties: nil},
			ComponentDefs: nil,
			TraitDefs:     nil,
			Error:         "have empty properties",
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			compDefs, traitDefs, err := ParseOverridePolicyRelatedDefinitions(context.Background(), cli, app, tt.Policy)
			if tt.Error != "" {
				r.NotNil(err)
				r.Contains(err.Error(), tt.Error)
			} else {
				r.NoError(err)
				r.Equal(len(tt.ComponentDefs), len(compDefs))
				for i := range tt.ComponentDefs {
					r.Equal(tt.ComponentDefs[i].Name, compDefs[i].Name)
					r.Equal(tt.ComponentDefs[i].Namespace, compDefs[i].Namespace)
				}
				r.Equal(len(tt.TraitDefs), len(traitDefs))
				for i := range tt.TraitDefs {
					r.Equal(tt.TraitDefs[i].Name, traitDefs[i].Name)
					r.Equal(tt.TraitDefs[i].Namespace, traitDefs[i].Namespace)
				}
			}
		})
	}
}
