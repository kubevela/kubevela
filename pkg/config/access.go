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
	"k8s.io/apiserver/pkg/endpoints/request"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ScopeSystem is the scope of a template whose configs live in the system namespace and are
// shared with every namespace.
const ScopeSystem = "system"

// ConfigAccess is the answer to "may an Application in one namespace act on configs in
// another". A nil Review is yes without a user: the same namespace, or a read of a shared system
// config. Otherwise the answer is the Application's user's, through Review, with Reason as the
// denial. Workflow steps run as the controller, so with no user to ask a review is a refusal.
type ConfigAccess struct {
	Review *authorizationv1.ResourceAttributes
	Reason string
}

// AccessToConfig answers for verb (get, list, create or delete) on the config name in namespace,
// from an Application in fromNamespace. sharedSystemConfig says the configs read are shared
// system configs (SharedSystemConfig), which any namespace may read but not write. A config is a
// Secret, so the review is of Secrets.
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
			Version:   "v1",
			Resource:  "secrets",
			Namespace: namespace,
			Name:      name,
		},
		Reason: fmt.Sprintf("%s is outside the application's namespace %s: it may %s it only if its user can (grant %s on secrets in namespace %s)",
			target, fromNamespace, verb, verb, namespace),
	}
}

// SharedSystemConfig reports whether the config name in the system namespace was made from a
// system-scoped template, as platform configs are, which makes it readable from any namespace. Only
// a KubeVela config counts: another Secret there with the scope label is not one.
func SharedSystemConfig(ctx context.Context, cli client.Client, name string) (bool, error) {
	var secret corev1.Secret
	if err := cli.Get(ctx, client.ObjectKey{Namespace: apitypes.DefaultKubeVelaNS, Name: name}, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return secret.Labels[apitypes.LabelConfigCatalog] == apitypes.VelaCoreConfig &&
		secret.Labels[apitypes.LabelConfigScope] == ScopeSystem, nil
}

// AllowConfigAccess applies AccessToConfig for a workflow step acting, as the controller, on
// configs in namespace for an Application in fromNamespace: allowed in that namespace, for a
// read of shared system configs, or with a named user in ctx (the AuthenticateApplication gate
// on) when that user may do the same there. name is empty for a list, which names the template
// whose configs it reads.
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
	// With the gate off the controller acts as itself and the user is only the Application's own
	// annotation, so there is no user to ask about.
	user, ok := request.UserFrom(ctx)
	if !ok || user == nil || user.GetName() == "" || cli == nil || !utilfeature.DefaultMutableFeatureGate.Enabled(features.AuthenticateApplication) {
		return fmt.Errorf("%s; this application runs with no user to ask for (the %s feature gate is off)", access.Reason, features.AuthenticateApplication)
	}
	sar := &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{
		User: user.GetName(), UID: user.GetUID(), Groups: user.GetGroups(), ResourceAttributes: access.Review,
	}}
	if err := cli.Create(asController, sar); err != nil {
		return err
	}
	if !sar.Status.Allowed {
		return fmt.Errorf("%s; %s may not", access.Reason, user.GetName())
	}
	return nil
}

// sharedSystemConfigs reports whether a read in the system namespace is of shared system
// configs: the named config, or for a list every config of the template.
func sharedSystemConfigs(ctx context.Context, cli client.Client, name, template string) (bool, error) {
	if name != "" {
		return SharedSystemConfig(ctx, cli, name)
	}
	tmpl, err := NewConfigFactory(cli).LoadTemplate(ctx, template, apitypes.DefaultKubeVelaNS)
	if errors.Is(err, ErrTemplateNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return tmpl.Scope == ScopeSystem, nil
}

// AllowStepConfigAccess applies AllowConfigAccess for a workflow step or a policy template, which
// run in the controller for the Application appLabels name. A write creates the config or
// overwrites it, so one outside that namespace needs both verbs. A caller with no Application
// namespace label runs outside the controller, the CLI or a velaql view, with its own client,
// and is not limited here.
func AllowStepConfigAccess(ctx context.Context, cli client.Client, appLabels map[string]string, verb, namespace, name, template string) error {
	from := appLabels[oam.LabelAppNamespace]
	if from == "" {
		return nil
	}
	verbs := []string{verb}
	if verb == "create" {
		verbs = append(verbs, "update")
	}
	for _, v := range verbs {
		if err := AllowConfigAccess(ctx, cli, v, namespace, name, template, from); err != nil {
			return err
		}
	}
	return nil
}

// StepListScope is the scope a step's list is narrowed to: a list of shared system configs, from
// another namespace, returns only system-scoped ones, as a read of one of them requires.
func StepListScope(ctx context.Context, cli client.Client, appLabels map[string]string, namespace, template string) (string, error) {
	from := appLabels[oam.LabelAppNamespace]
	if from == "" || namespace == from || namespace != apitypes.DefaultKubeVelaNS || cli == nil {
		return "", nil
	}
	shared, err := sharedSystemConfigs(auth.ContextClearUserInfo(ctx), cli, "", template)
	if err != nil || !shared {
		return "", err
	}
	return ScopeSystem, nil
}

// StepReadContext is what a read AllowStepConfigAccess allowed is made with: outside the
// Application's namespace, the controller's own identity, since access was already decided (a
// shared system config, or a user reviewed for it) and the user need not hold the read themselves.
func StepReadContext(ctx context.Context, appLabels map[string]string, namespace string) context.Context {
	if from := appLabels[oam.LabelAppNamespace]; from != "" && namespace != from {
		return auth.ContextClearUserInfo(ctx)
	}
	return ctx
}
