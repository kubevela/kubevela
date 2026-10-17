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
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
)

// kindServed reports whether the API server serves gvk (config.KindServed over the
// command's client). Call it once per command and reuse the result.
func kindServed(f velacmd.Factory, gvk schema.GroupVersionKind) bool {
	return config.KindServed(f.Client().RESTMapper(), gvk)
}

var (
	errConfigTemplateCRDMissing = errors.New("the ConfigTemplate CRD is not installed; upgrade vela-core before applying config templates")
	errConfigCRDMissing         = errors.New("the Config CRD is not installed; upgrade vela-core before creating configs")
)

// legacyTemplateExists reports whether a legacy template ConfigMap with the given
// template name exists in ns.
func legacyTemplateExists(ctx context.Context, cli client.Client, ns, name string) (bool, error) {
	return config.ObjectExists(ctx, cli, client.ObjectKey{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + name}, &corev1.ConfigMap{})
}

// legacyTemplateMigrated reports whether the legacy template name in ns was
// adopted by migration: its ConfigMap names a ConfigTemplate UID (migratedTo, the
// migrated-to annotation) and that CR still exists in the same namespace. Once
// the CR is gone the ConfigMap counts as plain legacy again, which is the
// downgrade story.
func legacyTemplateMigrated(ctx context.Context, cli client.Client, ns, name, migratedTo string) (bool, error) {
	if migratedTo == "" {
		return false, nil
	}
	var ct configv1alpha1.ConfigTemplate
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &ct); err != nil {
		if apierrors.IsNotFound(err) || config.IsCRDTypeMissing(err) {
			return false, nil
		}
		return false, err
	}
	return string(ct.UID) == migratedTo, nil
}

// legacyTemplateShadowed reports whether a legacy template ConfigMap for name
// exists in ns that migration has not adopted, so a ConfigTemplate CR of that
// name hides it rather than standing for it.
func legacyTemplateShadowed(ctx context.Context, cli client.Client, ns, name string) (bool, error) {
	var cm corev1.ConfigMap
	exists, err := config.ObjectExists(ctx, cli, client.ObjectKey{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + name}, &cm)
	if err != nil || !exists {
		return false, err
	}
	migrated, err := legacyTemplateMigrated(ctx, cli, ns, name, cm.Annotations[types.AnnotationConfigMigratedTo])
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
	// the webhook's rule, answered before anything is written and where the webhooks are off
	if config.SystemScopeMisplaced(ns, scope) {
		return config.SystemScopePlacementError(ns, t.Name)
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
