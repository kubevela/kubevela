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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// The helpers the CLI's create, delete and list commands and the workflow's config
// steps share: the kind probe, the Config CR built from a spec, its direct write, the
// existence check, the CR list, the all-backends list and the routing delete.

func TestKindServed(t *testing.T) {
	r := require.New(t)
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(configv1alpha1.ConfigGroupVersionKind, meta.RESTScopeNamespace)
	r.True(KindServed(mapper, configv1alpha1.ConfigGroupVersionKind))
	r.False(KindServed(mapper, configv1alpha1.ConfigTemplateGroupVersionKind), "the two CRDs are probed one by one")
}

func TestBuildConfigCR(t *testing.T) {
	tmpl := &NamespacedName{Name: "nw-plain", Namespace: "wf-a"}

	t.Run("plain template: properties inline", func(t *testing.T) {
		r := require.New(t)
		cfg, secret, err := BuildConfigCR(ConfigCRSpec{Name: "c", Namespace: "wf-a", Template: tmpl, Properties: map[string]interface{}{"value": "v"}, Alias: "a", Description: "d"})
		r.NoError(err)
		r.Nil(secret)
		r.Equal("c", cfg.Name)
		r.Equal("wf-a", cfg.Namespace)
		r.Equal(&configv1alpha1.ConfigTemplateReference{Name: "nw-plain", Namespace: "wf-a"}, cfg.Spec.TemplateRef)
		r.JSONEq(`{"value":"v"}`, string(cfg.Spec.Properties.Raw))
		r.Nil(cfg.Spec.PropertiesFrom)
		r.Equal("a", cfg.Spec.Alias)
		r.Equal("d", cfg.Spec.Description)
	})

	t.Run("sensitive template: properties in the companion secret", func(t *testing.T) {
		r := require.New(t)
		cfg, secret, err := BuildConfigCR(ConfigCRSpec{Name: "s", Namespace: "wf-a", Template: tmpl, Sensitive: true, Properties: map[string]interface{}{"password": "p"}})
		r.NoError(err)
		r.NotNil(secret)
		r.Equal("s-properties", secret.Name)
		r.Equal("wf-a", secret.Namespace)
		r.JSONEq(`{"password":"p"}`, string(secret.Data[DefaultPropertiesSecretKey]))
		r.Nil(cfg.Spec.Properties, "nothing readable on the Config")
		r.Equal("s-properties", cfg.Spec.PropertiesFrom.SecretRef.Name)
	})

	t.Run("no properties, no template", func(t *testing.T) {
		r := require.New(t)
		cfg, secret, err := BuildConfigCR(ConfigCRSpec{Name: "n", Namespace: "wf-a", Sensitive: true})
		r.NoError(err)
		r.Nil(secret)
		r.Nil(cfg.Spec.TemplateRef)
		r.Nil(cfg.Spec.Properties)
		r.Nil(cfg.Spec.PropertiesFrom)
	})
}

func TestUpsertConfigCR(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	cfg, secret, err := BuildConfigCR(ConfigCRSpec{Name: "s", Namespace: "wf-a", Sensitive: true, Properties: map[string]interface{}{"password": "p", "extra": "x"}})
	r.NoError(err)
	r.NoError(UpsertConfigCR(ctx, cli, cfg, secret))

	var got configv1alpha1.Config
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "s"}, &got))
	r.Equal("s-properties", got.Spec.PropertiesFrom.SecretRef.Name)
	var gotSecret corev1.Secret
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "s-properties"}, &gotSecret))
	r.JSONEq(`{"password":"p","extra":"x"}`, string(gotSecret.Data[DefaultPropertiesSecretKey]))

	// the second write replaces: the dropped key is gone, the spec is the new one
	cfg2, secret2, err := BuildConfigCR(ConfigCRSpec{Name: "s", Namespace: "wf-a", Sensitive: true, Properties: map[string]interface{}{"password": "q"}, Alias: "renamed"})
	r.NoError(err)
	r.NoError(UpsertConfigCR(ctx, cli, cfg2, secret2))
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "s-properties"}, &gotSecret))
	r.JSONEq(`{"password":"q"}`, string(gotSecret.Data[DefaultPropertiesSecretKey]))
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "s"}, &got))
	r.Equal("renamed", got.Spec.Alias)
	r.Equal("s", gotSecret.Labels[types.LabelConfigPropertiesOf], "the companion says whose it is")

	t.Run("a Secret of the companion's name that is not ours is refused, not overwritten", func(t *testing.T) {
		r := require.New(t)
		theirs := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "t-properties", Namespace: "wf-a"}, Data: map[string][]byte{"value": []byte("theirs")}}
		r.NoError(cli.Create(ctx, theirs))
		cfg, secret, err := BuildConfigCR(ConfigCRSpec{Name: "t", Namespace: "wf-a", Sensitive: true, Properties: map[string]interface{}{"password": "p"}})
		r.NoError(err)
		err = UpsertConfigCR(ctx, cli, cfg, secret)
		r.Error(err)
		r.Contains(err.Error(), "is not the properties secret of config t")
		var after corev1.Secret
		r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "t-properties"}, &after))
		r.Equal("theirs", string(after.Data["value"]))
		r.True(apierrors.IsNotFound(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "t"}, &configv1alpha1.Config{})), "nothing written")
	})

	t.Run("a spec with no properties removes the companion of ours", func(t *testing.T) {
		r := require.New(t)
		plain, none, err := BuildConfigCR(ConfigCRSpec{Name: "s", Namespace: "wf-a"})
		r.NoError(err)
		r.Nil(none)
		r.NoError(UpsertConfigCR(ctx, cli, plain, nil))
		r.True(apierrors.IsNotFound(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "s-properties"}, &corev1.Secret{})), "sensitive values do not outlive the spec that needed them")
		var got configv1alpha1.Config
		r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "s"}, &got))
		r.Nil(got.Spec.PropertiesFrom)
	})
}

func TestDeleteConfigRemovesTheCompanionSecret(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	cfg, secret, err := BuildConfigCR(ConfigCRSpec{Name: "d", Namespace: "wf-a", Sensitive: true, Properties: map[string]interface{}{"password": "p"}})
	r.NoError(err)
	r.NoError(UpsertConfigCR(ctx, cli, cfg, secret))
	// a user's Secret that happens to carry another config's companion name
	theirs := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "e-properties", Namespace: "wf-a"}, Data: map[string][]byte{"value": []byte("theirs")}}
	r.NoError(cli.Create(ctx, theirs))
	r.NoError(cli.Create(ctx, &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: "wf-a"}}))

	f := NewConfigFactory(cli)
	r.NoError(f.DeleteConfig(ctx, "wf-a", "d"))
	r.True(apierrors.IsNotFound(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "d"}, &configv1alpha1.Config{})))
	r.True(apierrors.IsNotFound(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "d-properties"}, &corev1.Secret{})), "the companion has no owner reference, so the delete takes it")

	r.NoError(f.DeleteConfig(ctx, "wf-a", "e"))
	var kept corev1.Secret
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "e-properties"}, &kept), "not ours: left alone")
}

func TestConfigCRExistsAndListConfigCRs(t *testing.T) {
	ctx := context.Background()
	ref := func(name string) *configv1alpha1.ConfigTemplateReference {
		return &configv1alpha1.ConfigTemplateReference{Name: name, Namespace: "wf-a"}
	}
	objects := []client.Object{
		&configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "wf-a"}, Spec: configv1alpha1.ConfigSpec{TemplateRef: ref("t")}},
		&configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "wf-a"}, Spec: configv1alpha1.ConfigSpec{TemplateRef: ref("other")}},
		&configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "wf-b"}, Spec: configv1alpha1.ConfigSpec{TemplateRef: ref("t")}},
		&configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "wf-b"}},
	}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(objects...).Build()

	t.Run("exists", func(t *testing.T) {
		r := require.New(t)
		ok, err := ConfigCRExists(ctx, cli, "wf-a", "a")
		r.NoError(err)
		r.True(ok)
		ok, err = ConfigCRExists(ctx, cli, "wf-a", "nope")
		r.NoError(err)
		r.False(ok)
	})

	t.Run("list by namespace and template", func(t *testing.T) {
		r := require.New(t)
		items, err := ListConfigCRs(ctx, cli, "wf-a", "")
		r.NoError(err)
		r.Len(items, 2)
		items, err = ListConfigCRs(ctx, cli, "", "t")
		r.NoError(err)
		r.Len(items, 2, "a and c reference t; d has no template")
		items, err = ListConfigCRs(ctx, cli, "wf-b", "t")
		r.NoError(err)
		r.Len(items, 1)
		r.Equal("c", items[0].Name)
	})

	t.Run("a cluster without the type", func(t *testing.T) {
		r := require.New(t)
		noConfig := runtime.NewScheme()
		r.NoError(clientgoscheme.AddToScheme(noConfig))
		bare := fake.NewClientBuilder().WithScheme(noConfig).Build()
		ok, err := ConfigCRExists(ctx, bare, "wf-a", "a")
		r.NoError(err)
		r.False(ok)
		items, err := ListConfigCRs(ctx, bare, "wf-a", "")
		r.NoError(err)
		r.Nil(items)
	})
}

// configSecret is a velacore-config Secret as the legacy CLI writes it, or as the Config
// controller materialises it when owner is set.
func configSecret(ns, name, template string, owner *configv1alpha1.Config, extraData map[string][]byte) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns,
			Labels:      map[string]string{types.LabelConfigCatalog: types.VelaCoreConfig, types.LabelConfigType: template},
			Annotations: map[string]string{types.AnnotationConfigTemplateNamespace: ns},
		},
		Data: map[string][]byte{SaveInputPropertiesKey: []byte(`{"value":"` + name + `"}`)},
	}
	for k, v := range extraData {
		s.Data[k] = v
	}
	if owner != nil {
		s.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: configv1alpha1.SchemeGroupVersion.String(), Kind: configv1alpha1.ConfigKind,
			Name: owner.Name, UID: owner.UID, Controller: ptr.To(true),
		}}
	}
	return s
}

func TestListAllConfigsSeesBothBackends(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cr := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "cr", Namespace: "wf-a", UID: "u1"}}
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(
		cr,
		configSecret("wf-a", "legacy", "t", nil, nil),
		configSecret("wf-a", "cr", "t", cr, nil),
		configSecret("wf-a", "other", "u", nil, nil),
	).Build()
	factory := NewConfigFactory(cli)

	legacyOnly, err := factory.ListConfigs(ctx, "wf-a", "t", "", false)
	r.NoError(err)
	r.Len(legacyOnly, 1, "ListConfigs keeps skipping the CR-owned Secret for the CLI's merged table")
	r.Equal("legacy", legacyOnly[0].Name)

	all, err := factory.ListAllConfigs(ctx, "wf-a", "t")
	r.NoError(err)
	names := []string{}
	for _, c := range all {
		names = append(names, c.Name)
	}
	r.ElementsMatch([]string{"legacy", "cr"}, names, "each config once, whichever backend wrote it")
	for _, c := range all {
		r.Equal(c.Name, c.Properties["value"], "the properties come from the Secret for both")
	}
}

func TestDeleteConfigRoutesByBackend(t *testing.T) {
	ctx := context.Background()

	t.Run("a Config CR of that name is deleted; its Secret is the owner reference's business", func(t *testing.T) {
		r := require.New(t)
		cr := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "cr", Namespace: "wf-a", UID: "u1"}}
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cr, configSecret("wf-a", "cr", "t", cr, nil)).Build()
		r.NoError(NewConfigFactory(cli).DeleteConfig(ctx, "wf-a", "cr"))
		ok, err := ConfigCRExists(ctx, cli, "wf-a", "cr")
		r.NoError(err)
		r.False(ok)
		r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "cr"}, &corev1.Secret{}), "no garbage collection in the fake client: the Secret is left to the API server")
	})

	t.Run("a legacy Secret and its referenced objects are deleted", func(t *testing.T) {
		r := require.New(t)
		out := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "legacy-out", Namespace: "wf-a"}}
		refs := []byte(`[{"kind":"ConfigMap","apiVersion":"v1","name":"legacy-out","namespace":"wf-a"}]`)
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(out, configSecret("wf-a", "legacy", "t", nil, map[string][]byte{SaveObjectReferenceKey: refs})).Build()
		r.NoError(NewConfigFactory(cli).DeleteConfig(ctx, "wf-a", "legacy"))
		r.Error(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "legacy"}, &corev1.Secret{}))
		r.Error(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "legacy-out"}, &corev1.ConfigMap{}))
	})

	t.Run("neither backend holds it", func(t *testing.T) {
		r := require.New(t)
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		r.EqualError(NewConfigFactory(cli).DeleteConfig(ctx, "wf-a", "nope"), "the config nope not found")
	})
}
