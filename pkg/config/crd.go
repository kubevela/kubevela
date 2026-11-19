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
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

// DefaultPropertiesSecretKey is the key of a Config's companion properties Secret: the
// one BuildConfigCR writes for a sensitive template and the one the controller reads
// when spec.propertiesFrom names no key.
const DefaultPropertiesSecretKey = "properties"

// KindServed reports whether the API server serves gvk. The Config and ConfigTemplate
// CRDs are installed separately from the controller in principle, so a writer probes
// the kind it is about to write and falls back to the legacy storage when it is absent.
func KindServed(mapper meta.RESTMapper, gvk schema.GroupVersionKind) bool {
	_, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}

// ConfigCRExists reports whether a Config CR with the given name exists in ns. A cluster
// that does not serve the type counts as "no".
func ConfigCRExists(ctx context.Context, cli client.Client, ns, name string) (bool, error) {
	return ObjectExists(ctx, cli, client.ObjectKey{Namespace: ns, Name: name}, &configv1alpha1.Config{})
}

// ListConfigCRs lists Config CRs in ns, or everywhere when ns is "", keeping only those
// referencing template when it is set. A cluster that does not serve the type yields no
// items and no error.
func ListConfigCRs(ctx context.Context, cli client.Client, ns, template string) ([]configv1alpha1.Config, error) {
	var list configv1alpha1.ConfigList
	var opts []client.ListOption
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := cli.List(ctx, &list, opts...); err != nil {
		if IsCRDTypeMissing(err) {
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

// ConfigCRSpec is what a Config CR is built from: the CLI's create command and the
// workflow's create-config step both describe a config this way.
type ConfigCRSpec struct {
	Name      string
	Namespace string
	// Template is the ConfigTemplate the config is made from; nil for a template-less config.
	Template *NamespacedName
	// Sensitive is the template's flag: the properties then go into a companion Secret
	// rather than onto the Config, where anyone who can read Configs would see them.
	Sensitive   bool
	Properties  map[string]interface{}
	Alias       string
	Description string
}

// BuildConfigCR returns the Config CR for spec and, for a sensitive template with
// properties, the companion Secret <name>-properties that spec.propertiesFrom points at.
// Nothing is written; the two objects are applied by UpsertConfigCR (direct) or
// Factory.ApplyConfigCR (through the factory's dispatcher).
func BuildConfigCR(spec ConfigCRSpec) (*configv1alpha1.Config, *corev1.Secret, error) {
	cfg := &configv1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace},
		Spec:       configv1alpha1.ConfigSpec{Alias: spec.Alias, Description: spec.Description},
	}
	if spec.Template != nil && spec.Template.Name != "" {
		cfg.Spec.TemplateRef = &configv1alpha1.ConfigTemplateReference{Name: spec.Template.Name, Namespace: spec.Template.Namespace}
	}
	if len(spec.Properties) == 0 {
		return cfg, nil, nil
	}
	raw, err := json.Marshal(spec.Properties)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to encode the properties of config %s: %w", spec.Name, err)
	}
	if !spec.Sensitive {
		cfg.Spec.Properties = &runtime.RawExtension{Raw: raw}
		return cfg, nil, nil
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: PropertiesSecretName(spec.Name), Namespace: spec.Namespace, Annotations: map[string]string{types.AnnotationConfigPropertiesOf: spec.Name}},
		Data:       map[string][]byte{DefaultPropertiesSecretKey: raw},
	}
	cfg.Spec.PropertiesFrom = &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: secret.Name}}
	return cfg, secret, nil
}

// PropertiesSecretName is the name of the companion Secret holding a sensitive Config's
// properties.
func PropertiesSecretName(configName string) string { return configName + "-properties" }

// IsPropertiesSecretOf reports a companion Secret written for the named Config, by the CLI,
// the workflow step or migration: the only Secret of that name they may overwrite or
// delete. A user's own Secret that happens to carry the name is nobody's to touch.
func IsPropertiesSecretOf(secret *corev1.Secret, configName string) bool {
	return secret.Annotations[types.AnnotationConfigPropertiesOf] == configName
}

// DeletePropertiesSecretOf removes the companion Secret of the named Config, if one of
// ours stands there; a foreign Secret of the name, or none, is left alone.
func DeletePropertiesSecretOf(ctx context.Context, cli client.Client, namespace, configName string) error {
	var secret corev1.Secret
	if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: PropertiesSecretName(configName)}, &secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !IsPropertiesSecretOf(&secret, configName) {
		return nil
	}
	return client.IgnoreNotFound(cli.Delete(ctx, &secret))
}

// UpsertConfigCR creates or updates the Config and its companion Secret, if any, directly
// through the client: the spec is replaced, so a property the caller dropped is dropped.
// The CLI's write path. A companion Secret is written only where none stands or one of
// ours for this Config does (IsPropertiesSecretOf); a Secret of that name that is not ours
// is refused rather than overwritten. A companion of ours left over from a sensitive spec
// is removed when the new spec has none.
func UpsertConfigCR(ctx context.Context, cli client.Client, cfg *configv1alpha1.Config, properties *corev1.Secret) error {
	if properties != nil {
		existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: properties.Name, Namespace: properties.Namespace}}
		if _, err := controllerutil.CreateOrUpdate(ctx, cli, existing, func() error {
			if existing.ResourceVersion != "" && !IsPropertiesSecretOf(existing, cfg.Name) { // stored already (a fake client sets no UID)
				return fmt.Errorf("secret %s/%s exists and is not the properties secret of config %s", existing.Namespace, existing.Name, cfg.Name)
			}
			if existing.Annotations == nil {
				existing.Annotations = map[string]string{}
			}
			for k, v := range properties.Annotations {
				existing.Annotations[k] = v
			}
			existing.Data = properties.Data
			return nil
		}); err != nil {
			return fmt.Errorf("fail to write the properties secret %s: %w", properties.Name, err)
		}
	} else if err := DeletePropertiesSecretOf(ctx, cli, cfg.Namespace, cfg.Name); err != nil {
		return fmt.Errorf("fail to remove the properties secret of config %s: %w", cfg.Name, err)
	}
	existing := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name, Namespace: cfg.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, cli, existing, func() error {
		existing.Spec = cfg.Spec
		return nil
	})
	return err
}

// IsCRDTypeMissing reports whether err means a config.oam.dev type is not served
// by the API server or not registered in the client scheme. Dual-backend code
// treats that the same as a CR that does not exist: a cluster that was never
// upgraded has legacy objects and nothing else.
func IsCRDTypeMissing(err error) bool {
	return meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)
}

// SetDistributionOwner makes the Config the controller owner of its distribution
// Application, so deleting the Config (via the CLI, kubectl, or GitOps removing the
// manifest) always recalls the distributed resources instead of orphaning the
// Application and the copies it manages in target namespaces/clusters.
// A config may still be legacy (a plain Secret created by an older CLI), and the
// cluster may not serve the Config type at all, so a missing Config CR here is
// expected and not an error - there's just no owner to attach.
func SetDistributionOwner(ctx context.Context, cli client.Client, ns, configName, distributionName string, opts ...client.UpdateOption) error {
	cfg := &configv1alpha1.Config{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: configName}, cfg); err != nil {
		if apierrors.IsNotFound(err) || IsCRDTypeMissing(err) {
			return nil
		}
		return fmt.Errorf("failed to load config %s to own its distribution: %w", configName, err)
	}
	// the Application controller reconciles (and updates status on) the distribution
	// concurrently, so the update below can lose an optimistic-concurrency race; retry.
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		app := &v1beta1.Application{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: distributionName}, app); err != nil {
			return err
		}
		if metav1.IsControlledBy(app, cfg) {
			return nil
		}
		if err := controllerutil.SetControllerReference(cfg, app, cli.Scheme()); err != nil {
			return err
		}
		return cli.Update(ctx, app, opts...)
	})
	if err != nil {
		return fmt.Errorf("failed to set owner reference on distribution %s: %w", distributionName, err)
	}
	return nil
}

// ParseTemplateRef splits a template given as "name" or "namespace/name"; the namespace
// defaults to vela-system, as the CLI and the workflow steps have always read it.
func ParseTemplateRef(template string) NamespacedName {
	ref := NamespacedName{Name: template, Namespace: types.DefaultKubeVelaNS}
	if ns, name, ok := strings.Cut(template, "/"); ok {
		ref.Namespace, ref.Name = ns, name
	}
	return ref
}

// ObjectExists reports whether obj can be read at key. Not found, and a type the cluster
// does not serve, both count as absent.
func ObjectExists(ctx context.Context, cli client.Client, key client.ObjectKey, obj client.Object) (bool, error) {
	err := cli.Get(ctx, key, obj)
	if apierrors.IsNotFound(err) || IsCRDTypeMissing(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
