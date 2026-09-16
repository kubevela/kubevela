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

// registryNameLister reads the names of the configured addon registries. It is
// a seam for tests; production uses component.ListRegistryNames.
type registryNameLister func(context.Context, client.Client) ([]string, error)

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

	listRegistryNames registryNameLister
}

// NewValidator creates a Validator reading registry names through cli.
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
// chooseVersion for a Helm or OCI registry and in checkVersionPinSupported for a
// git or OSS one, and nothing requires a git registry's metadata.yaml version to
// be semver. Rejecting a non-semver pin here would therefore refuse Applications
// that resolve today.
type componentProperties struct {
	Addon    string `json:"addon"`
	Version  string `json:"version"`
	Registry string `json:"registry"`
}

// ValidateComponents rejects type: addon components that cannot resolve for a
// reason visible locally: properties that do not decode, or a registry that is
// not configured on this cluster.
func (v *Validator) ValidateComponents(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	startTime := time.Now()
	logger := logging.WithContext(ctx).WithStep("validate-addon-components")
	logger.Info("Addon component validation started")

	addonComponentCount := 0
	var errs field.ErrorList
	var registryNames []string
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
			names, err := v.readRegistryNames(ctx)
			if err != nil {
				// Fail open, and stop asking. The registry name may well be
				// correct and this webhook runs with failurePolicy: Fail, so a
				// ConfigMap read that failed for its own reasons must not block
				// the apply. The renderer reports an unknown registry at
				// reconcile time.
				logger.Info("Skipping addon registry name check", "reason", "registry-list-failed", "error", err)
			} else {
				registryNames, registriesKnown = names, true
			}
			registriesRead = true
		}
		if registriesKnown && !slices.Contains(registryNames, properties.Registry) {
			// field.NotFound would render as `Not found: "typo"`, which leaves
			// the author guessing what the right value was. Registry names are
			// not sensitive, and the list is exactly what they need to see.
			errs = append(errs, field.Invalid(path.Child("registry"), properties.Registry,
				fmt.Sprintf("is not a configured addon registry; configured: %v", registryNames)))
		}
	}

	logger.WithSuccess(len(errs) == 0, startTime).Info(
		"Addon component validation completed",
		"addonComponentCount", addonComponentCount,
		"errorCount", len(errs),
	)
	return errs
}

func (v *Validator) readRegistryNames(ctx context.Context) ([]string, error) {
	list := v.listRegistryNames
	if list == nil {
		list = component.ListRegistryNames
	}
	return list(ctx, v.Client)
}
