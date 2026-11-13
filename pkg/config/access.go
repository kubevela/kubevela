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
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
)

// ConfigAccess is the answer to "may an Application in one namespace act on configs in
// another". A nil Review is yes without a user: the same namespace, or a read of a shared system
// config. Otherwise the answer is the Application's user's, through Review, with Reason as the
// denial. Workflow steps and sources run as the controller, so with no user to ask a review is
// a refusal.
type ConfigAccess struct {
	Review *authorizationv1.ResourceAttributes
	Reason string
}

// AccessToConfig answers for verb (get, list, create or delete) on the config name in namespace,
// from an Application in fromNamespace. sharedSystemConfig says the configs read are shared
// system configs (SharedSystemConfig), which any namespace may read but not write.
func AccessToConfig(verb, namespace, name, fromNamespace string, sharedSystemConfig bool) ConfigAccess {
	if namespace == fromNamespace {
		return ConfigAccess{}
	}
	read := verb == "get" || verb == "list"
	if read && sharedSystemConfig && namespace == apitypes.DefaultKubeVelaNS {
		return ConfigAccess{}
	}
	target := fmt.Sprintf("config %s/%s", namespace, name)
	if name == "" {
		target = "configs in namespace " + namespace
	}
	return ConfigAccess{
		Review: &authorizationv1.ResourceAttributes{
			Verb:      verb,
			Group:     configv1alpha1.Group,
			Version:   configv1alpha1.Version,
			Resource:  "configs",
			Namespace: namespace,
			Name:      name,
		},
		Reason: fmt.Sprintf("%s is outside the application's namespace %s: it may %s it only if its user can (grant %s on configs.%s in namespace %s)",
			target, fromNamespace, verb, verb, configv1alpha1.Group, namespace),
	}
}

// SharedSystemConfig reports whether the config name in the system namespace was made from a
// system-scoped template, as platform configs are, which makes it readable from any namespace.
func SharedSystemConfig(ctx context.Context, cli client.Client, name string) (bool, error) {
	key := client.ObjectKey{Namespace: apitypes.DefaultKubeVelaNS, Name: name}
	var ref *configv1alpha1.ConfigTemplateReference
	var cfg configv1alpha1.Config
	switch err := cli.Get(ctx, key, &cfg); {
	case err == nil:
		ref = cfg.Spec.TemplateRef
	case apierrors.IsNotFound(err), IsCRDTypeMissing(err):
		var secret corev1.Secret
		if err := cli.Get(ctx, key, &secret); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if t := secret.Labels[apitypes.LabelConfigType]; t != "" {
			ref = &configv1alpha1.ConfigTemplateReference{Name: t}
		}
	default:
		return false, err
	}
	if ref == nil {
		return false, nil
	}
	tmpl, _, err := TemplateScopeOf(ctx, cli, ref)
	if errors.Is(err, ErrTemplateNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return tmpl.EffectiveScope() == configv1alpha1.ConfigTemplateScopeSystem, nil
}

// AllowConfigAccess applies AccessToConfig for a workflow step or source acting, as the
// controller, on configs in namespace for an Application in fromNamespace: allowed in that
// namespace, for a read of shared system configs, or with a named user in ctx (the
// AuthenticateApplication gate on) when that user may do the same there. name is empty for a
// list, which names the template whose configs it reads.
func AllowConfigAccess(ctx context.Context, cli client.Client, verb, namespace, name, template, fromNamespace string) error {
	if namespace == fromNamespace {
		return nil
	}
	asController := auth.ContextClearUserInfo(ctx)
	shared := false
	if (verb == "get" || verb == "list") && namespace == apitypes.DefaultKubeVelaNS && cli != nil {
		var err error
		if shared, err = sharedSystemConfigs(asController, cli, name, template); err != nil {
			return err
		}
	}
	access := AccessToConfig(verb, namespace, name, fromNamespace, shared)
	if access.Review == nil {
		return nil
	}
	if cli != nil && !KindServed(cli.RESTMapper(), configv1alpha1.ConfigGroupVersionKind) {
		// With no Config kind a config is a legacy Secret, so the user is asked about Secrets.
		access = legacyAccess(access, verb, namespace, fromNamespace)
	}
	// With the gate off the controller acts as itself and the user is only the Application's own
	// annotation, so there is no user to ask about.
	user, named := auth.NamedUserFrom(ctx)
	if !named || !utilfeature.DefaultMutableFeatureGate.Enabled(features.AuthenticateApplication) {
		return fmt.Errorf("%s; this application runs with no user to ask for (the %s feature gate is off)", access.Reason, features.AuthenticateApplication)
	}
	allowed, err := auth.UserCan(asController, cli, auth.UserInfoOf(user), *access.Review)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%s; %s may not", access.Reason, user.GetName())
	}
	return nil
}

// legacyAccess turns a review of Configs into the same review of the Secrets legacy configs are.
func legacyAccess(access ConfigAccess, verb, namespace, fromNamespace string) ConfigAccess {
	review := *access.Review
	review.Group, review.Version, review.Resource = "", "v1", "secrets"
	target := fmt.Sprintf("config %s/%s", namespace, review.Name)
	if review.Name == "" {
		target = "configs in namespace " + namespace
	}
	return ConfigAccess{
		Review: &review,
		Reason: fmt.Sprintf("%s is outside the application's namespace %s: it may %s it only if its user can (grant %s on secrets in namespace %s)",
			target, fromNamespace, verb, verb, namespace),
	}
}

// sharedSystemConfigs reports whether a read in the system namespace is of shared system
// configs: the named config, or for a list every config of the template.
func sharedSystemConfigs(ctx context.Context, cli client.Client, name, template string) (bool, error) {
	if name != "" {
		return SharedSystemConfig(ctx, cli, name)
	}
	tmpl, _, err := TemplateScopeOf(ctx, cli, &configv1alpha1.ConfigTemplateReference{Name: ParseTemplateRef(template).Name})
	if errors.Is(err, ErrTemplateNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return tmpl.EffectiveScope() == configv1alpha1.ConfigTemplateScopeSystem, nil
}
