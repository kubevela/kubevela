/*
Copyright 2026 The KubeVela Authors.

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

package addon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func rawProps(t *testing.T, m map[string]interface{}) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

func TestValidateComponents(t *testing.T) {
	testCases := map[string]struct {
		components   []common.ApplicationComponent
		checker      func(calls *int) func(ctx context.Context, addon, version, registry string) *field.Error
		wantErrCount int
		wantField    string
		wantCalls    int
	}{
		"compatible addon component produces no error": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "version": "1.0.0"})},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, _, _, _ string) *field.Error { *calls++; return nil }
			},
			wantErrCount: 0,
			wantCalls:    1,
		},
		"incompatible addon component yields one field error on the properties path": {
			components: []common.ApplicationComponent{
				{Name: "webservice-comp", Type: "webservice"},
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd"})},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, _, _, _ string) *field.Error {
					*calls++
					return field.Invalid(field.NewPath("x"), "fluxcd", "requires kubernetes >= 1.30")
				}
			},
			wantErrCount: 1,
			wantField:    "spec.components[1].properties",
			wantCalls:    1,
		},
		"skipVersionValidate short-circuits before the checker": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "skipVersionValidate": true})},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, _, _, _ string) *field.Error {
					*calls++
					return field.Invalid(field.NewPath("x"), "fluxcd", "should never be reached")
				}
			},
			wantErrCount: 0,
			wantCalls:    0,
		},
		"malformed properties skip compatibility validation": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": map[string]interface{}{"not": "a string"}})},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, _, _, _ string) *field.Error {
					*calls++
					return field.Invalid(field.NewPath("x"), "fluxcd", "should never be reached")
				}
			},
			wantErrCount: 0,
			wantCalls:    0,
		},
		"fail-open checker registry error yields no denial": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd"})},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, _, _, _ string) *field.Error { *calls++; return nil }
			},
			wantErrCount: 0,
			wantCalls:    1,
		},
		"non-addon components are ignored": {
			components: []common.ApplicationComponent{
				{Name: "comp1", Type: "webservice"},
				{Name: "comp2", Type: "worker"},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, _, _, _ string) *field.Error {
					*calls++
					return field.Invalid(field.NewPath("x"), "x", "should never be reached")
				}
			},
			wantErrCount: 0,
			wantCalls:    0,
		},
		"addon name defaults to component name when addon field empty": {
			components: []common.ApplicationComponent{
				{Name: "velaux", Type: ComponentType},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, addon, _, _ string) *field.Error {
					*calls++
					if addon != "velaux" {
						return field.Invalid(field.NewPath("x"), addon, "unexpected addon name")
					}
					return nil
				}
			},
			wantErrCount: 0,
			wantCalls:    1,
		},
		"multiple addon components are checked independently": {
			components: []common.ApplicationComponent{
				{Name: "first", Type: ComponentType},
				{Name: "second", Type: ComponentType},
			},
			checker: func(calls *int) func(context.Context, string, string, string) *field.Error {
				return func(_ context.Context, _, _, _ string) *field.Error { *calls++; return nil }
			},
			wantErrCount: 0,
			wantCalls:    2,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			validator := &Validator{compatChecker: tc.checker(&calls)}
			app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: tc.components}}

			errs := validator.ValidateComponents(context.Background(), app)

			assert.Len(t, errs, tc.wantErrCount)
			assert.Equal(t, tc.wantCalls, calls, "unexpected checker invocation count")
			if tc.wantField != "" {
				require.Len(t, errs, 1)
				assert.Equal(t, tc.wantField, errs[0].Field)
			}
		})
	}
}

func TestValidateComponentsForwardsProperties(t *testing.T) {
	calls := 0
	validator := &Validator{compatChecker: func(_ context.Context, addonName, version, registry string) *field.Error {
		calls++
		assert.Equal(t, "fluxcd", addonName)
		assert.Equal(t, "2.0.0", version)
		assert.Equal(t, "KubeVela", registry)
		return field.Invalid(field.NewPath("requirements"), addonName, "incompatible")
	}}
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		{Name: "api", Type: "webservice"},
		{Name: "installer", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{
			"addon": "fluxcd", "version": "2.0.0", "registry": "KubeVela",
		})},
	}}}

	errs := validator.ValidateComponents(context.Background(), app)
	require.Len(t, errs, 1)
	assert.Equal(t, 1, calls)
	assert.Equal(t, "spec.components[1].properties", errs[0].Field)
}
