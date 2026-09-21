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

package config

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

type errGetClient struct {
	client.Client
	err    error
	failOn client.Object
}

func (e *errGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	switch obj.(type) {
	case *configv1alpha1.ConfigTemplate:
		if _, ok := e.failOn.(*configv1alpha1.ConfigTemplate); ok {
			return e.err
		}
	case *corev1.ConfigMap:
		if _, ok := e.failOn.(*corev1.ConfigMap); ok {
			return e.err
		}
	}
	return e.Client.Get(ctx, key, obj, opts...)
}

func TestResolveConfigTemplate(t *testing.T) {
	r := require.New(t)

	t.Run("CRD available, default namespace", func(t *testing.T) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "helm-repo", Namespace: apitypes.DefaultKubeVelaNS},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template:  "template: {}",
				Scope:     configv1alpha1.ConfigTemplateScopeNamespace,
				Sensitive: true,
			},
			Status: configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "helm-repo"})
		r.NoError(err)
		r.False(waiting)
		r.NotNil(tmpl)
		r.Equal("helm-repo", tmpl.Name)
		r.Equal(apitypes.DefaultKubeVelaNS, tmpl.Namespace)
		r.Equal(string(configv1alpha1.ConfigTemplateScopeNamespace), tmpl.Scope)
		r.True(tmpl.Sensitive)
	})

	t.Run("CRD found but not yet available", func(t *testing.T) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "custom-ns"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: "template: {}"},
			Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseError},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "pending", Namespace: "custom-ns"})
		r.NoError(err)
		r.True(waiting)
		r.Nil(tmpl)
	})

	t.Run("no CRD, falls back to legacy ConfigMap", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TemplateConfigMapNamePrefix + "legacy",
				Namespace: apitypes.DefaultKubeVelaNS,
				Labels:    map[string]string{apitypes.LabelConfigScope: "system"},
				Annotations: map[string]string{
					apitypes.AnnotationConfigSensitive: sensitiveAnnotationValue,
				},
			},
			Data: map[string]string{SaveTemplateKey: "template: {}"},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cm).Build()

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "legacy"})
		r.NoError(err)
		r.False(waiting)
		r.NotNil(tmpl)
		r.Equal("legacy", tmpl.Name)
		r.Equal(apitypes.DefaultKubeVelaNS, tmpl.Namespace)
		r.Equal("system", tmpl.Scope)
		r.True(tmpl.Sensitive)
		r.Equal("template: {}", string(tmpl.CUE))
	})

	t.Run("neither CRD nor legacy ConfigMap exist", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "missing"})
		r.ErrorIs(err, ErrTemplateNotFound)
		r.False(waiting)
		r.Nil(tmpl)
	})

	t.Run("unexpected error getting the ConfigTemplate CRD is returned as-is", func(t *testing.T) {
		boom := errors.New("boom")
		cli := &errGetClient{
			Client: fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			err:    boom,
			failOn: &configv1alpha1.ConfigTemplate{},
		}

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "any"})
		r.ErrorIs(err, boom)
		r.False(waiting)
		r.Nil(tmpl)
	})

	t.Run("unexpected error getting the legacy ConfigMap is returned as-is", func(t *testing.T) {
		boom := errors.New("boom")
		cli := &errGetClient{
			Client: fake.NewClientBuilder().WithScheme(common.Scheme).Build(),
			err:    boom,
			failOn: &corev1.ConfigMap{},
		}

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "any"})
		r.ErrorIs(err, boom)
		r.False(waiting)
		r.Nil(tmpl)
	})
}
