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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
)

const defaultPropertiesSecretKey = "properties"

// configCRDInstalled reports whether the config.oam.dev CRDs are served by the
// API server. Call it once per command and reuse the result.
func configCRDInstalled(f velacmd.Factory) bool {
	_, err := f.Client().RESTMapper().RESTMapping(configv1alpha1.ConfigGroupVersionKind.GroupKind(), configv1alpha1.Version)
	return err == nil
}

var errConfigTemplateCRDMissing = errors.New("the ConfigTemplate CRD is not installed; upgrade vela-core before applying config templates")

// objectExists reports whether obj can be read at key. Not found, and a type
// the cluster does not serve, both count as absent.
func objectExists(ctx context.Context, cli client.Client, key client.ObjectKey, obj client.Object) (bool, error) {
	err := cli.Get(ctx, key, obj)
	if apierrors.IsNotFound(err) || crdTypeMissing(err) {
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

// crdTypeMissing reports whether err means the config.oam.dev types are not served
// by the API server or not registered in the client scheme. Dual-backend paths treat
// that the same as a CR that doesn't exist.
func crdTypeMissing(err error) bool {
	return meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)
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
		if apierrors.IsNotFound(err) || crdTypeMissing(err) {
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

// listConfigTemplateCRDs lists ConfigTemplate CRs in ns, or everywhere when ns is
// "". A cluster that does not serve the type yields no items and no error.
func listConfigTemplateCRDs(ctx context.Context, cli client.Client, ns string) ([]configv1alpha1.ConfigTemplate, error) {
	var list configv1alpha1.ConfigTemplateList
	var opts []client.ListOption
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := cli.List(ctx, &list, opts...); err != nil {
		if crdTypeMissing(err) {
			return nil, nil
		}
		return nil, err
	}
	return list.Items, nil
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
		if crdTypeMissing(err) {
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

// setDistributionOwner makes the Config the controller owner of its distribution
// Application, so deleting the Config (via the CLI, kubectl, or GitOps removing the
// manifest) always recalls the distributed resources instead of orphaning the
// Application and the copies it manages in target namespaces/clusters.
// A config may still be legacy (a plain Secret, created by an older CLI or by the
// Nacos interim path), and the cluster may not serve the Config type at all, so a
// missing Config CR here is expected and not an error - there's just no owner to attach.
func setDistributionOwner(ctx context.Context, cli client.Client, ns, configName, distributionName string) error {
	cfg := &configv1alpha1.Config{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: configName}, cfg); err != nil {
		if apierrors.IsNotFound(err) || crdTypeMissing(err) {
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
		return cli.Update(ctx, app)
	})
	if err != nil {
		return fmt.Errorf("failed to set owner reference on distribution %s: %w", distributionName, err)
	}
	return nil
}
