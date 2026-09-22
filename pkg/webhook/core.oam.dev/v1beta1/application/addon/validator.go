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

// Package addon holds the admission-time checks for type: addon Application
// components. Everything here is answerable from the Application and the
// cluster; resolving the addon from its registry belongs to the controller.
package addon

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// ComponentType is the ComponentDefinition name used by the addon-as-component
// feature: an Application component of this type installs an addon.
const ComponentType = "addon"

// registryLister reads what the validator needs to know about the configured
// addon registries: every name, and which of those names are Git backed.
//
// One seam returning both, rather than one per fact, so a Validator cannot be
// built with half of it stubbed and silently reach the cluster for the other
// half.
//
// It is a seam for tests; production uses listClusterRegistries.
type registryLister func(context.Context, client.Client) (names []string, gitKinds map[string]string, err error)

// listClusterRegistries is the production registryLister: one read of the
// addon registry ConfigMap, and no token Secret.
//
// That read reaches the apiserver rather than an informer. ConfigMaps are in
// the manager client's uncached set by default (the
// DisableWorkflowContextConfigMapCache gate), so this costs one GET inside the
// admission timeout. It is one GET, once per request, and only for an
// Application that names a registry -- see the skip in ValidateComponents.
func listClusterRegistries(ctx context.Context, cli client.Client) ([]string, map[string]string, error) {
	return component.ListRegistrySources(ctx, cli)
}

// Validator validates addon-specific Application components.
//
// Everything it checks is answerable from the Application itself plus the
// registry ConfigMap. It deliberately does not resolve the addon from its
// registry: that is a listing plus one request per file in the package, per
// addon component, against a host the cluster does not control, while the
// apiserver holds the admission request open under admissionWebhookTimeout and
// failurePolicy: Fail. The addon's SystemRequirements are checked instead where
// the package is actually fetched, by the renderer at reconcile time, and a
// mismatch surfaces in the Application status.
type Validator struct {
	// Client reads the addon registry ConfigMap. It comes from the manager
	// registering this webhook rather than a process-wide singleton, because
	// the webhook process never initializes that singleton: relying on it
	// means every call falls back to its lazy loader, which builds a
	// brand-new, uncached client from ambient kubeconfig state (and panics
	// outright with none).
	Client client.Client

	listRegistries registryLister
}

// NewValidator creates a Validator reading the registry records through cli.
func NewValidator(cli client.Client) *Validator {
	return &Validator{Client: cli}
}

// componentProperties is the subset of a type: addon component's properties
// this validator can check without contacting the registry.
// Addon and Version are decoded but not otherwise inspected. They stay on the
// struct because decoding is itself the check: a component that writes a map
// for addon or a number for version fails to decode and is rejected above.
//
// Version deliberately gets no format check. Pinned-version resolution compares
// the requested version to the package's own version as a string, both in
// chooseVersion for a Helm or OCI registry and in checkVersionPinSupported for
// an OSS one, and nothing requires an OSS registry's metadata.yaml version to
// be semver. Rejecting a non-semver pin here would therefore refuse Applications
// that resolve today.
type componentProperties struct {
	Addon    string `json:"addon"`
	Version  string `json:"version"`
	Registry string `json:"registry"`
}

// ValidateComponents rejects type: addon components that cannot resolve for a
// reason visible locally: properties that do not decode, a registry that is
// not configured on this cluster, or a registry backed by Git.
//
// Git is refused here rather than at render because the answer is in the
// registry ConfigMap, so admission can give it immediately and precisely,
// while a render-time refusal would surface as a failing Application the
// author has to go and read. `vela addon enable` is unaffected: this checks
// Application components, not the registry record, and a Git registry stays
// installable imperatively.
//
// A component that names no registry is not checked for this. An empty
// registry means "search every configured registry", and which one wins is
// decided by reading each in turn over the network -- exactly what admission
// must not do. The renderer refuses those.
func (v *Validator) ValidateComponents(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	startTime := time.Now()
	logger := logging.WithContext(ctx).WithStep("validate-addon-components")
	logger.Debug("Addon component validation started")

	addonComponentCount := 0
	var errs field.ErrorList
	var registryNames []string
	var registryGitKinds map[string]string
	// Tracked separately from registryNames: a cluster with no registries
	// configured yields an empty list, which must still reject a named
	// registry, whereas a failed read must not reject anything.
	registriesRead, registriesKnown := false, false

	for i, comp := range app.Spec.Components {
		if comp.Type != ComponentType {
			continue
		}
		addonComponentCount++
		path := field.NewPath("spec", "components").Index(i).Child("properties")

		properties := componentProperties{}
		if comp.Properties != nil && len(comp.Properties.Raw) > 0 {
			if err := json.Unmarshal(comp.Properties.Raw, &properties); err != nil {
				// Reject rather than skip. The Application itself is malformed
				// (a non-string addon, a numeric version), the render will fail
				// later anyway, and admitting it silently is the one outcome
				// that leaves the author with no idea why.
				logger.Error(err, "Rejecting malformed addon component properties", "component", comp.Name)
				errs = append(errs, field.Invalid(path, string(comp.Properties.Raw),
					"cannot be decoded as addon component properties: "+err.Error()))
				continue
			}
		}

		if properties.Registry == "" {
			continue
		}
		if !registriesRead {
			names, kinds, err := v.readRegistries(ctx)
			if err != nil {
				// Fail open, and stop asking. The registry name may well be
				// correct and this webhook runs with failurePolicy: Fail, so a
				// ConfigMap read that failed for its own reasons must not block
				// the apply. The renderer reports an unknown registry, and
				// refuses a Git one, at reconcile time.
				logger.Info("Skipping addon registry checks", "reason", "registry-list-failed", "error", err)
			} else {
				registryNames, registryGitKinds, registriesKnown = names, kinds, true
			}
			registriesRead = true
		}
		if !registriesKnown {
			continue
		}
		if !slices.Contains(registryNames, properties.Registry) {
			// field.NotFound would render as `Not found: "typo"`, which leaves
			// the author guessing what the right value was. Registry names are
			// not sensitive, and the list is exactly what they need to see.
			errs = append(errs, field.Invalid(path.Child("registry"), properties.Registry,
				fmt.Sprintf("is not a configured addon registry; configured: %v", registryNames)))
			continue
		}
		if kind := registryGitKinds[properties.Registry]; kind != "" {
			errs = append(errs, field.Invalid(path.Child("registry"), properties.Registry,
				component.GitSourceUnsupportedDetail(kind, component.AddonGitRemedy)))
		}
	}

	logger.WithSuccess(len(errs) == 0, startTime).Info(
		"Addon component validation completed",
		"addonComponentCount", addonComponentCount,
		"errorCount", len(errs),
	)
	return errs
}

func (v *Validator) readRegistries(ctx context.Context) ([]string, map[string]string, error) {
	list := v.listRegistries
	if list == nil {
		list = listClusterRegistries
	}
	return list(ctx, v.Client)
}
