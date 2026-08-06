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

package cli

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
)

// defaultPropertiesSecretKey matches the key ConfigTemplateReference.PropertiesFrom
// defaults to when spec.propertiesFrom.secretRef.key is omitted.
const defaultPropertiesSecretKey = "properties"

// configCRDAvailable reports whether the config.oam.dev CRDs are installed on the
// cluster. Callers fall back to the legacy ConfigMap/Secret-based Factory when they
// aren't, so old clusters/addons keep working unmodified.
func configCRDAvailable(f velacmd.Factory) bool {
	_, err := f.Client().RESTMapper().RESTMapping(configv1alpha1.ConfigGroupVersionKind.GroupKind(), configv1alpha1.Version)
	return err == nil
}

func applyConfigTemplateCRD(ctx context.Context, cli client.Client, ns string, t *config.Template) error {
	// the legacy convention allows any scope string (e.g. "project"), but the CRD's
	// spec.scope only accepts "system"/"namespace"; only "system" carries distinct
	// meaning, so anything else maps to "namespace".
	scope := configv1alpha1.ConfigTemplateScopeNamespace
	if t.Scope == string(configv1alpha1.ConfigTemplateScopeSystem) {
		scope = configv1alpha1.ConfigTemplateScopeSystem
	}

	ct := &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Name: t.Name, Namespace: ns}}
	_, err := controllerutil.CreateOrUpdate(ctx, cli, ct, func() error {
		ct.Spec = configv1alpha1.ConfigTemplateSpec{
			Template:    string(t.Template),
			Scope:       scope,
			Sensitive:   t.Sensitive,
			Alias:       t.Alias,
			Description: t.Description,
		}
		return nil
	})
	return err
}

func deleteConfigTemplateCRD(ctx context.Context, cli client.Client, ns, name string) error {
	ct := &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if err := cli.Delete(ctx, ct); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("the config template %s not found", name)
		}
		return err
	}
	return nil
}

func listConfigTemplateCRDs(ctx context.Context, cli client.Client, ns string) ([]configv1alpha1.ConfigTemplate, error) {
	var list configv1alpha1.ConfigTemplateList
	var opts []client.ListOption
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := cli.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func deleteConfigCRD(ctx context.Context, cli client.Client, ns, name string) error {
	cfg := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if err := cli.Delete(ctx, cfg); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("the config %s not found", name)
		}
		return err
	}
	return nil
}

func listConfigCRDs(ctx context.Context, cli client.Client, ns, template string) ([]configv1alpha1.Config, error) {
	var list configv1alpha1.ConfigList
	var opts []client.ListOption
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := cli.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	if template == "" {
		return list.Items, nil
	}
	filtered := make([]configv1alpha1.Config, 0, len(list.Items))
	for _, c := range list.Items {
		if c.Spec.TemplateRef != nil && c.Spec.TemplateRef.Name == template {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}

// createConfigCRD creates or updates a Config CRD from the `vela config create` options.
// If the resolved template is Sensitive, properties are written to a companion
// Secret and referenced via spec.propertiesFrom instead of being embedded inline in
// the Config, matching the CRD design (spec.properties is documented as
// non-sensitive; propertiesFrom exists precisely so secret material never lands in
// a plainly-readable object).
func createConfigCRD(ctx context.Context, cli client.Client, ns, name, templateName, templateNamespace string, sensitive bool, properties map[string]interface{}, alias, description string) error {
	spec := configv1alpha1.ConfigSpec{
		Alias:       alias,
		Description: description,
	}
	if templateName != "" {
		spec.TemplateRef = &configv1alpha1.ConfigTemplateReference{Name: templateName, Namespace: templateNamespace}
	}
	if len(properties) > 0 {
		raw, err := json.Marshal(properties)
		if err != nil {
			return err
		}
		if sensitive {
			secretName := name + "-properties"
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns}}
			if _, err := controllerutil.CreateOrUpdate(ctx, cli, secret, func() error {
				secret.Data = map[string][]byte{defaultPropertiesSecretKey: raw}
				return nil
			}); err != nil {
				return err
			}
			spec.PropertiesFrom = &configv1alpha1.PropertiesReference{
				SecretRef: configv1alpha1.SecretKeySelector{Name: secretName},
			}
		} else {
			spec.Properties = &runtime.RawExtension{Raw: raw}
		}
	}

	cfg := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	_, err := controllerutil.CreateOrUpdate(ctx, cli, cfg, func() error {
		cfg.Spec = spec
		return nil
	})
	return err
}
