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
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
)

const defaultPropertiesSecretKey = "properties"

// kindServed reports whether the API server serves gvk. The Config and
// ConfigTemplate CRDs are installed separately, so each command probes the kind
// it is about to write. Call it once per command and reuse the result.
func kindServed(f velacmd.Factory, gvk schema.GroupVersionKind) bool {
	_, err := f.Client().RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}

var (
	errConfigTemplateCRDMissing = errors.New("the ConfigTemplate CRD is not installed; upgrade vela-core before applying config templates")
	errConfigCRDMissing         = errors.New("the Config CRD is not installed; upgrade vela-core before creating configs")
)

// objectExists reports whether obj can be read at key. Not found, and a type
// the cluster does not serve, both count as absent.
func objectExists(ctx context.Context, cli client.Client, key client.ObjectKey, obj client.Object) (bool, error) {
	err := cli.Get(ctx, key, obj)
	if apierrors.IsNotFound(err) || config.IsCRDTypeMissing(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// legacyTemplateExists reports whether a legacy template ConfigMap with the given
// template name exists in ns.
func legacyTemplateExists(ctx context.Context, cli client.Client, ns, name string) (bool, error) {
	return objectExists(ctx, cli, client.ObjectKey{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + name}, &corev1.ConfigMap{})
}

// legacyTemplateMigrated reports whether cm, a legacy template ConfigMap, was
// adopted by migration: it names a ConfigTemplate UID and that CR still exists in
// the same namespace. Once the CR is gone the ConfigMap counts as plain legacy
// again, which is the downgrade story.
func legacyTemplateMigrated(ctx context.Context, cli client.Client, cm *corev1.ConfigMap) (bool, error) {
	uid := migratedToUID(cm)
	if uid == "" {
		return false, nil
	}
	name := strings.TrimPrefix(cm.Name, config.TemplateConfigMapNamePrefix)
	var ct configv1alpha1.ConfigTemplate
	if err := cli.Get(ctx, client.ObjectKey{Namespace: cm.Namespace, Name: name}, &ct); err != nil {
		if apierrors.IsNotFound(err) || config.IsCRDTypeMissing(err) {
			return false, nil
		}
		return false, err
	}
	return string(ct.UID) == uid, nil
}

// migratedToUID returns the ConfigTemplate UID a legacy template ConfigMap says
// adopted it, or "" when it was never migrated.
func migratedToUID(cm *corev1.ConfigMap) string {
	if cm == nil {
		return ""
	}
	return cm.Annotations[types.AnnotationConfigMigratedTo]
}

// legacyTemplateShadowed reports whether a legacy template ConfigMap for name
// exists in ns that migration has not adopted, so a ConfigTemplate CR of that
// name hides it rather than standing for it.
func legacyTemplateShadowed(ctx context.Context, cli client.Client, ns, name string) (bool, error) {
	var cm corev1.ConfigMap
	exists, err := objectExists(ctx, cli, client.ObjectKey{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + name}, &cm)
	if err != nil || !exists {
		return false, err
	}
	migrated, err := legacyTemplateMigrated(ctx, cli, &cm)
	if err != nil {
		return false, err
	}
	return !migrated, nil
}

func applyConfigTemplateCRD(ctx context.Context, cli client.Client, ns string, t *config.Template) error {
	// legacy scope strings (e.g. "project") map to "namespace"; only "system" carries distinct meaning
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

// deleteConfigTemplateCRD deletes the ConfigTemplate CR and reports whether one
// was there. No CR, or a cluster that does not serve the type, is false and no
// error, so the caller can fall back to the legacy ConfigMap.
func deleteConfigTemplateCRD(ctx context.Context, cli client.Client, ns, name string) (bool, error) {
	ct := &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if err := cli.Delete(ctx, ct); err != nil {
		if apierrors.IsNotFound(err) || config.IsCRDTypeMissing(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// configCRDExists reports whether a Config CR with the given name exists in ns.
// A cluster that does not serve the type counts as no CR.
func configCRDExists(ctx context.Context, cli client.Client, ns, name string) (bool, error) {
	return objectExists(ctx, cli, client.ObjectKey{Namespace: ns, Name: name}, &configv1alpha1.Config{})
}

// listAllConfigTemplateCRDs lists every ConfigTemplate CR in ns, or everywhere
// when ns is "", generated ones included. A cluster that does not serve the type
// yields no items and no error.
func listAllConfigTemplateCRDs(ctx context.Context, cli client.Client, ns string) ([]configv1alpha1.ConfigTemplate, error) {
	var list configv1alpha1.ConfigTemplateList
	var opts []client.ListOption
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := cli.List(ctx, &list, opts...); err != nil {
		if config.IsCRDTypeMissing(err) {
			return nil, nil
		}
		return nil, err
	}
	return list.Items, nil
}

// userManagedConfigTemplates drops the templates generated from a
// SourceDefinition, as Factory.ListTemplates drops them: they are the
// application controller's cache, not something a user lists or edits.
func userManagedConfigTemplates(items []configv1alpha1.ConfigTemplate) []configv1alpha1.ConfigTemplate {
	kept := make([]configv1alpha1.ConfigTemplate, 0, len(items))
	for _, ct := range items {
		if _, generated := ct.Labels[types.LabelSourceDefinitionName]; generated {
			continue
		}
		kept = append(kept, ct)
	}
	return kept
}

// deleteConfigCRD deletes the Config CR. Callers route here only after
// configCRDExists said yes, so a missing CR is reported as not found.
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

// listConfigCRDs lists Config CRs in ns, or everywhere when ns is "", keeping
// only those referencing template when it is set. A cluster that does not serve
// the type yields no items and no error.
func listConfigCRDs(ctx context.Context, cli client.Client, ns, template string) ([]configv1alpha1.Config, error) {
	var list configv1alpha1.ConfigList
	var opts []client.ListOption
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := cli.List(ctx, &list, opts...); err != nil {
		if config.IsCRDTypeMissing(err) {
			return nil, nil
		}
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

// createConfigCRD creates or updates a Config CRD. Sensitive template properties are
// written to a companion Secret and referenced via spec.propertiesFrom instead of
// being embedded inline.
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
