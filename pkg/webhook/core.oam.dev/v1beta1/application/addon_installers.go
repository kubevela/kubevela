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

package application

import (
	"context"
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	addonvalidation "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/application/addon"
)

// ValidateAddonInstallers refuses an addon component whose author may not install addons. The
// controller installs the addon with its own identity: the addon's Application in the system
// namespace and whatever cluster-scoped resources it holds. Creating Applications in the system
// namespace is what installing an addon takes, so the author must be able to do that themselves.
//
// An Application the controller writes is judged for the one that dispatched it: one dispatched
// from the system namespace (an addon's own Application, installing what it depends on) may carry
// addon components; one dispatched from elsewhere is checked as that Application's author where
// the identity is trusted, and refused otherwise. One the controller writes for itself, outside
// dispatch, is not checked.
func (h *ValidatingHandler) ValidateAddonInstallers(ctx context.Context, app *v1beta1.Application, req admission.Request) field.ErrorList {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableAddonComponent) {
		return nil
	}
	var paths []*field.Path
	for i, comp := range app.Spec.Components {
		if comp.Type == addonvalidation.ComponentType {
			paths = append(paths, field.NewPath("spec", "components").Index(i).Child("type"))
		}
	}
	for _, use := range indirectReach(ctx, h.Client, app).uses {
		if use.defType == addonvalidation.ComponentType {
			paths = append(paths, use.path)
		}
	}
	if len(paths) == 0 {
		return nil
	}

	author := req.UserInfo
	if h.fromController(req) {
		dispatcher := dispatchingNamespace(app)
		if dispatcher == "" || dispatcher == oam.SystemDefinitionNamespace {
			return nil
		}
		trustedAuthor, trusted, err := h.dispatcherAuthor(ctx, app)
		if err != nil {
			return forbidAll(paths, func(p *field.Path) *field.Error { return field.InternalError(p, err) })
		}
		if !trusted {
			return forbidAll(paths, func(p *field.Path) *field.Error {
				return field.Forbidden(p, fmt.Sprintf("an addon component can be dispatched only from an Application in namespace %q", oam.SystemDefinitionNamespace))
			})
		}
		author = trustedAuthor
	}
	return h.mayInstallAddons(ctx, author, paths)
}

// mayInstallAddons refuses at paths unless user may create Applications in the system namespace.
func (h *ValidatingHandler) mayInstallAddons(ctx context.Context, user authenticationv1.UserInfo, paths []*field.Path) field.ErrorList {
	allowed, err := auth.UserCan(ctx, h.Client, user, authv1.ResourceAttributes{
		Verb: "create", Group: "core.oam.dev", Version: "v1beta1", Resource: "applications", Namespace: oam.SystemDefinitionNamespace,
	})
	switch {
	case err != nil:
		return forbidAll(paths, func(p *field.Path) *field.Error {
			return field.InternalError(p, fmt.Errorf("cannot check whether %q may install addons: %w", user.Username, err))
		})
	case !allowed:
		return forbidAll(paths, func(p *field.Path) *field.Error {
			return field.Forbidden(p, fmt.Sprintf("installing an addon needs the right to create Applications in namespace %q, which user %q does not have",
				oam.SystemDefinitionNamespace, user.Username))
		})
	}
	return nil
}

func forbidAll(paths []*field.Path, errFor func(*field.Path) *field.Error) field.ErrorList {
	errs := make(field.ErrorList, 0, len(paths))
	for _, p := range paths {
		errs = append(errs, errFor(p))
	}
	return errs
}
