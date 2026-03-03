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

package application

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// mustRaw marshals v to a *runtime.RawExtension, panicking on error.
func mustRaw(t *testing.T, v map[string]any) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

func TestCheckImmutableParams(t *testing.T) {
	const tmplWithImmutable = `
parameter: {
	// +immutable
	image: string
	replicas: int
}`

	fp := field.NewPath("spec", "components").Index(0).Child("properties")

	cases := []struct {
		name         string
		template     string
		oldProps     *runtime.RawExtension
		newProps     *runtime.RawExtension
		wantErrCount int
	}{
		{
			name:         "no immutable fields in template",
			template:     `parameter: { image: string }`,
			oldProps:     mustRaw(t, map[string]any{"image": "v1"}),
			newProps:     mustRaw(t, map[string]any{"image": "v2"}),
			wantErrCount: 0,
		},
		{
			name:         "immutable field unchanged",
			template:     tmplWithImmutable,
			oldProps:     mustRaw(t, map[string]any{"image": "v1", "replicas": 2}),
			newProps:     mustRaw(t, map[string]any{"image": "v1", "replicas": 3}),
			wantErrCount: 0,
		},
		{
			name:         "immutable field changed",
			template:     tmplWithImmutable,
			oldProps:     mustRaw(t, map[string]any{"image": "v1"}),
			newProps:     mustRaw(t, map[string]any{"image": "v2"}),
			wantErrCount: 1,
		},
		{
			name:         "immutable field removed",
			template:     tmplWithImmutable,
			oldProps:     mustRaw(t, map[string]any{"image": "v1"}),
			newProps:     mustRaw(t, map[string]any{}),
			wantErrCount: 1,
		},
		{
			name:         "immutable field first set (was absent before)",
			template:     tmplWithImmutable,
			oldProps:     mustRaw(t, map[string]any{}),
			newProps:     mustRaw(t, map[string]any{"image": "v1"}),
			wantErrCount: 0,
		},
		{
			name:         "nil old props",
			template:     tmplWithImmutable,
			oldProps:     nil,
			newProps:     mustRaw(t, map[string]any{"image": "v1"}),
			wantErrCount: 0,
		},
		{
			name: "nested immutable field unchanged",
			template: `parameter: { governance: { // +immutable
tenant: string } }`,
			oldProps:     mustRaw(t, map[string]any{"governance": map[string]any{"tenant": "acme"}}),
			newProps:     mustRaw(t, map[string]any{"governance": map[string]any{"tenant": "acme"}}),
			wantErrCount: 0,
		},
		{
			name: "nested immutable field changed",
			template: `
parameter: {
	governance: {
		// +immutable
		tenant: string
		region: string
	}
}`,
			oldProps:     mustRaw(t, map[string]any{"governance": map[string]any{"tenant": "acme", "region": "us-east"}}),
			newProps:     mustRaw(t, map[string]any{"governance": map[string]any{"tenant": "other", "region": "us-east"}}),
			wantErrCount: 1,
		},
		{
			name: "nested mutable sibling field can change",
			template: `
parameter: {
	governance: {
		// +immutable
		tenant: string
		region: string
	}
}`,
			oldProps:     mustRaw(t, map[string]any{"governance": map[string]any{"tenant": "acme", "region": "us-east"}}),
			newProps:     mustRaw(t, map[string]any{"governance": map[string]any{"tenant": "acme", "region": "eu-west"}}),
			wantErrCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := checkImmutableParams(tc.template, fp, tc.oldProps, tc.newProps)
			assert.Len(t, errs, tc.wantErrCount)
		})
	}
}

func TestValidateImmutableFields(t *testing.T) {
	const immutableTemplate = `
parameter: {
	// +immutable
	image: string
	replicas: int
}`

	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	// Seed a ComponentDefinition with an immutable field in vela-system
	compDef := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "webservice",
			Namespace: "vela-system",
		},
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: immutableTemplate},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(compDef).
		Build()

	handler := &ValidatingHandler{Client: fakeClient}
	ctx := context.Background()

	makeApp := func(image string, annotations map[string]string) *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-app",
				Namespace:   "default",
				Annotations: annotations,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "comp1",
						Type:       "webservice",
						Properties: mustRaw(t, map[string]any{"image": image, "replicas": 3}),
					},
				},
			},
		}
	}

	t.Run("immutable field unchanged - no error", func(t *testing.T) {
		errs := handler.ValidateImmutableFields(ctx, makeApp("v1", nil), makeApp("v1", nil))
		assert.Empty(t, errs)
	})

	t.Run("immutable field changed - error", func(t *testing.T) {
		errs := handler.ValidateImmutableFields(ctx, makeApp("v2", nil), makeApp("v1", nil))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Detail, `immutable field cannot be changed (current: "v1", new: "v2")`)
	})

	t.Run("force-param-mutations bypasses all checks", func(t *testing.T) {
		errs := handler.ValidateImmutableFields(ctx,
			makeApp("v2", map[string]string{"app.oam.dev/force-param-mutations": "true"}),
			makeApp("v1", nil),
		)
		assert.Empty(t, errs)
	})

	t.Run("unknown definition type - skip gracefully", func(t *testing.T) {
		newApp := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "comp1",
						Type:       "unknown-type",
						Properties: mustRaw(t, map[string]any{"image": "v2"}),
					},
				},
			},
		}
		oldApp := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "comp1",
						Type:       "unknown-type",
						Properties: mustRaw(t, map[string]any{"image": "v1"}),
					},
				},
			},
		}
		errs := handler.ValidateImmutableFields(ctx, newApp, oldApp)
		assert.Empty(t, errs)
	})
}
