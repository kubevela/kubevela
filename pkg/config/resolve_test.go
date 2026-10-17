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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config/writer"
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
		r.Equal(&ResolvedTemplate{Name: "pending", Namespace: "custom-ns"}, tmpl, "name, namespace and scope only")
	})

	t.Run("a misplaced system template whose schema is extracted resolves, as namespace scope", func(t *testing.T) {
		r := require.New(t)
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "misplaced", Namespace: "custom-ns"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: "template: {}", Scope: configv1alpha1.ConfigTemplateScopeSystem},
			Status: configv1alpha1.ConfigTemplateStatus{
				Phase:  configv1alpha1.ConfigTemplatePhaseError,
				Schema: &runtime.RawExtension{Raw: []byte(`{}`)},
			},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "misplaced", Namespace: "custom-ns"})
		r.NoError(err)
		r.False(waiting)
		r.NotNil(tmpl)
		r.Equal(string(configv1alpha1.ConfigTemplateScopeSystem), tmpl.Scope)
		r.Equal(configv1alpha1.ConfigTemplateScopeNamespace, tmpl.EffectiveScope())
	})

	t.Run("a misplaced system template without a schema is still waiting", func(t *testing.T) {
		r := require.New(t)
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "misplaced-broken", Namespace: "custom-ns"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: "template: {{{", Scope: configv1alpha1.ConfigTemplateScopeSystem},
			Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseError},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "misplaced-broken", Namespace: "custom-ns"})
		r.NoError(err)
		r.True(waiting)
		r.Equal(configv1alpha1.ConfigTemplateScopeNamespace, tmpl.EffectiveScope(), "misplaced: read as namespace even while pending")
	})

	t.Run("the ConfigTemplate kind not served: the legacy ConfigMap, as LoadTemplate reads it", func(t *testing.T) {
		r := require.New(t)
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: TemplateConfigMapNamePrefix + "old", Namespace: apitypes.DefaultKubeVelaNS, Labels: map[string]string{apitypes.LabelConfigScope: "project"}},
			Data:       map[string]string{SaveTemplateKey: "template: {}"},
		}
		base := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cm).Build()
		cli := interceptor.NewClient(base, interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*configv1alpha1.ConfigTemplate); ok {
					return &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: configv1alpha1.Group, Kind: configv1alpha1.ConfigTemplateKind}}
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})
		tmpl, waiting, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "old"})
		r.NoError(err)
		r.False(waiting)
		r.True(tmpl.Legacy)
		r.Equal(configv1alpha1.ConfigTemplateScopeSystem, tmpl.EffectiveScope())
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

	t.Run("a template without a nacos block has no expanded writer", func(t *testing.T) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: apitypes.DefaultKubeVelaNS},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: "template: { output: {}, parameter: {} }"},
			Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, _, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "plain"})
		r.NoError(err)
		r.Nil(tmpl.ExpandedWriter.Nacos)
	})

	t.Run("the nacos block travels with a CRD template", func(t *testing.T) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "nc-app", Namespace: "nw-nacos"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: nacosTemplateCUE},
			Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, _, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "nc-app", Namespace: "nw-nacos"})
		r.NoError(err)
		r.NotNil(tmpl.ExpandedWriter.Nacos)
		r.Equal("nacos", tmpl.ExpandedWriter.Nacos.Endpoint.Name)
		r.Equal(apitypes.DefaultKubeVelaNS, tmpl.ExpandedWriter.Nacos.Endpoint.Namespace)
		r.Equal("properties", tmpl.ExpandedWriter.Nacos.Format)
	})

	t.Run("the nacos block travels with a legacy ConfigMap template", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: TemplateConfigMapNamePrefix + "nc-app", Namespace: "nw-nacos"},
			Data:       map[string]string{SaveTemplateKey: nacosTemplateCUE},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cm).Build()

		tmpl, _, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "nc-app", Namespace: "nw-nacos"})
		r.NoError(err)
		r.NotNil(tmpl.ExpandedWriter.Nacos)
		r.Equal("nacos", tmpl.ExpandedWriter.Nacos.Endpoint.Name)
		r.Equal("properties", tmpl.ExpandedWriter.Nacos.Format)
	})

	t.Run("a template whose CUE does not compile has no expanded writer and still resolves", func(t *testing.T) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: apitypes.DefaultKubeVelaNS},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: "template: { nacos: { endpoint: name: } }"},
			Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, _, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "broken"})
		r.NoError(err)
		r.Nil(tmpl.ExpandedWriter.Nacos)
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

	t.Run("a template that imports a vela package keeps its nacos block", func(t *testing.T) {
		r := require.New(t)
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "nc-validated", Namespace: "nw-nacos"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: nacosTemplateWithImportCUE},
			Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
		}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()

		tmpl, _, err := ResolveConfigTemplate(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "nc-validated", Namespace: "nw-nacos"})
		r.NoError(err)
		r.NotNil(tmpl.ExpandedWriter.Nacos, "the plain CUE compiler rejects the import; the CueX parse must keep the block")
		r.Equal("nacos", tmpl.ExpandedWriter.Nacos.Endpoint.Name)
	})
}

// nacosTemplateWithImportCUE imports a vela package, as the shipped image-registry
// template does for its validation. Only the CueX compiler can parse it.
const nacosTemplateWithImportCUE = `
import "vela/config"

template: {
	validation: config.#ImageRegistry & {
		$params: registry: "index.docker.io"
	}
	nacos: {
		endpoint: name: "nacos"
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  "DEFAULT_GROUP"
		}
		content: parameter.content
	}
	output: {}
	parameter: {
		dataId:  string
		content: {...}
	}
}
`

// nacosTemplateCUE is the shape of testdata/mysql-db-nacos.cue: a concrete endpoint, metadata
// from the parameter, properties content.
const nacosTemplateCUE = `
template: {
	nacos: {
		endpoint: name: "nacos"
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      context.name
			namespace: context.namespace
		}
	}
	parameter: {
		dataId: string
		group:  *"DEFAULT_GROUP" | string
		content: {
			mysqlHost: string
			mysqlPort: int
		}
	}
}
`

func TestEffectiveScope(t *testing.T) {
	system, namespace := configv1alpha1.ConfigTemplateScopeSystem, configv1alpha1.ConfigTemplateScopeNamespace
	for name, tc := range map[string]struct {
		tmpl ResolvedTemplate
		want configv1alpha1.ConfigTemplateScope
	}{
		"CR system in vela-system":                      {ResolvedTemplate{Namespace: "vela-system", Scope: "system"}, system},
		"CR namespace in vela-system stays namespace":   {ResolvedTemplate{Namespace: "vela-system", Scope: "namespace"}, namespace},
		"CR system outside vela-system reads namespace": {ResolvedTemplate{Namespace: "wf-a", Scope: "system"}, namespace},
		"CR namespace elsewhere":                        {ResolvedTemplate{Namespace: "wf-a", Scope: "namespace"}, namespace},
		"legacy in vela-system, whatever the label":     {ResolvedTemplate{Namespace: "vela-system", Scope: "project", Legacy: true}, system},
		"legacy in vela-system labelled namespace":      {ResolvedTemplate{Namespace: "vela-system", Scope: "namespace", Legacy: true}, system},
		"legacy elsewhere labelled global":              {ResolvedTemplate{Namespace: "wf-b", Scope: "global", Legacy: true}, namespace},
		"legacy elsewhere labelled system":              {ResolvedTemplate{Namespace: "wf-b", Scope: "system", Legacy: true}, namespace},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.tmpl.EffectiveScope())
		})
	}
}

func TestSystemScopeMisplaced(t *testing.T) {
	r := require.New(t)
	r.True(SystemScopeMisplaced("wf-a", configv1alpha1.ConfigTemplateScopeSystem))
	r.False(SystemScopeMisplaced("vela-system", configv1alpha1.ConfigTemplateScopeSystem))
	r.False(SystemScopeMisplaced("wf-a", configv1alpha1.ConfigTemplateScopeNamespace))
	r.Contains(SystemScopePlacementError("wf-a", "t").Error(), "config template wf-a/t: scope system is only valid in vela-system")
}

func TestPendingTemplateError(t *testing.T) {
	r := require.New(t)
	r.NoError(PendingTemplateError(nil, "wf-a", "t", "wf-a"), "its own namespace waits for a template that is not there yet")
	r.Error(PendingTemplateError(nil, "wf-b", "t", "wf-a"), "another namespace: nothing to tell the scope by")
	r.NoError(PendingTemplateError(&ResolvedTemplate{Name: "t", Namespace: apitypes.DefaultKubeVelaNS, Scope: "system"}, apitypes.DefaultKubeVelaNS, "t", "wf-a"), "a system template in vela-system is known from its spec")
	r.Error(PendingTemplateError(&ResolvedTemplate{Name: "t", Namespace: apitypes.DefaultKubeVelaNS, Scope: "namespace"}, apitypes.DefaultKubeVelaNS, "t", "wf-a"))
	r.Error(PendingTemplateError(&ResolvedTemplate{Name: "t", Namespace: "wf-b", Scope: "system"}, "wf-b", "t", "wf-a"), "misplaced: namespace scope")
}

func TestAccessToTemplate(t *testing.T) {
	r := require.New(t)
	same := AccessToTemplate(&ResolvedTemplate{Name: "a", Namespace: "wf-a", Scope: "namespace"}, "wf-a")
	r.Nil(same.Review, "the same namespace needs no user")

	system := AccessToTemplate(&ResolvedTemplate{Name: "s", Namespace: "vela-system", Scope: "system"}, "wf-a")
	r.Nil(system.Review, "a system template is usable from any namespace")

	legacySystem := AccessToTemplate(&ResolvedTemplate{Name: "l", Namespace: "vela-system", Scope: "project", Legacy: true}, "wf-a")
	r.Nil(legacySystem.Review, "a legacy template in vela-system is system whatever its label")

	cross := AccessToTemplate(&ResolvedTemplate{Name: "b", Namespace: "wf-b", Scope: "namespace"}, "wf-a")
	r.NotNil(cross.Review)
	r.NotNil(cross.Review)
	r.Equal("get", cross.Review.Verb)
	r.Equal("config.oam.dev", cross.Review.Group)
	r.Equal("configtemplates", cross.Review.Resource)
	r.Equal("wf-b", cross.Review.Namespace)
	r.Equal("b", cross.Review.Name)
	r.Contains(cross.Reason, "config template wf-b/b is namespace-scoped")
	r.Contains(cross.Reason, "grant get on configtemplates.config.oam.dev b in namespace wf-b")

	misplaced := AccessToTemplate(&ResolvedTemplate{Name: "m", Namespace: "wf-a", Scope: "system"}, "wf-b")
	r.NotNil(misplaced.Review, "system outside vela-system grants nothing beyond its namespace")

	legacyCross := AccessToTemplate(&ResolvedTemplate{Name: "lb", Namespace: "wf-b", Scope: "global", Legacy: true}, "wf-a")
	r.NotNil(legacyCross.Review)
	r.Equal("configtemplates", legacyCross.Review.Resource, "the review names the template the user would be granted, not the ConfigMap")
}

func TestTemplateScopeOf(t *testing.T) {
	r := require.New(t)
	ct := &configv1alpha1.ConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "wf-b", Annotations: map[string]string{AnnotationGrandfatheredConfigs: "wf-a/x, wf-c/y,"}},
		Spec:       configv1alpha1.ConfigTemplateSpec{Template: "this is not even cue {{{", Scope: configv1alpha1.ConfigTemplateScopeNamespace},
		Status:     configv1alpha1.ConfigTemplateStatus{Phase: configv1alpha1.ConfigTemplatePhaseAvailable},
	}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(ct).Build()
	tmpl, waiting, err := TemplateScopeOf(context.Background(), cli, &configv1alpha1.ConfigTemplateReference{Name: "old", Namespace: "wf-b"})
	r.NoError(err, "no CUE is compiled")
	r.False(waiting)
	r.Equal([]string{"wf-a/x", "wf-c/y"}, tmpl.Grandfathered)
	r.True(tmpl.Grandfathers(client.ObjectKey{Namespace: "wf-a", Name: "x"}))
	r.False(tmpl.Grandfathers(client.ObjectKey{Namespace: "wf-a", Name: "z"}), "a binding made after the adoption")
	r.False(tmpl.Grandfathers(client.ObjectKey{Namespace: "wf-b", Name: "x"}))
	r.Empty(tmpl.CUE)
}

func TestRenderedEndpointError(t *testing.T) {
	r := require.New(t)
	named := func(ns string) writer.ConfigRef { return writer.ConfigRef{Name: "nacos", Namespace: ns} }
	cr := &ResolvedTemplate{Name: "t", Namespace: "wf-a", ExpandedWriter: writer.ExpandedWriterConfig{Nacos: &writer.NacosConfig{Endpoint: named("wf-b")}}}
	r.NoError(RenderedEndpointError(cr, named("wf-a")), "its own namespace")
	r.NoError(RenderedEndpointError(cr, named(apitypes.DefaultKubeVelaNS)), "the shared endpoints")
	r.NoError(RenderedEndpointError(cr, named("wf-b")), "the endpoint the text names, which admission reviewed")
	err := RenderedEndpointError(cr, named("wf-c"))
	r.ErrorContains(err, "the nacos endpoint wf-c/nacos is outside wf-a and vela-system, and is not the one the template names")

	legacy := &ResolvedTemplate{Name: "t", Namespace: "wf-a", Legacy: true, ExpandedWriter: cr.ExpandedWriter}
	r.Error(RenderedEndpointError(legacy, named("wf-b")), "a legacy ConfigMap was never reviewed: no foreign endpoint, even the one it names")
	r.NoError(RenderedEndpointError(legacy, named("wf-a")))
}
