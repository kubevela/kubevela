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

// Package module holds the admission-time checks for type: module Application
// components. Everything here is answerable from the Application and the module
// registry ConfigMap; fetching the module from its registry belongs to the
// controller.
package module

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
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// ComponentType is the ComponentDefinition name used by the module-as-component
// feature: an Application component of this type installs a module.
const ComponentType = "module"

// registryLister reads what the validator needs to know about the configured
// module registries: every name, and which of those names are Git backed. A
// seam for tests; production uses listClusterRegistries.
type registryLister func(context.Context, client.Client) (names []string, gitKinds map[string]string, err error)

// listClusterRegistries is the production registryLister. It reads the
// module registry ConfigMap and no token Secret.
func listClusterRegistries(ctx context.Context, cli client.Client) ([]string, map[string]string, error) {
	return component.ListRegistrySourcesIn(ctx, cli, pkgmodule.ModuleRegistryConfigMap)
}

// Validator validates module-specific Application components.
//
// It refuses a component that names a Git-backed registry. Modules resolve from
// an OCI registry only, and the source kind is in the registry ConfigMap, so
// admission can answer that immediately rather than leaving the author to read
// a failing Application later.
//
// It deliberately does not fetch the module. That is an OCI pull against a host
// the cluster does not control, while the apiserver holds the admission request
// open under admissionWebhookTimeout and failurePolicy: Fail.
type Validator struct {
	// Client reads the module registry ConfigMap. It comes from the manager
	// registering this webhook rather than a process-wide singleton, for the
	// reason spelled out on the addon Validator: the webhook process never
	// initializes that singleton.
	Client client.Client

	listRegistries registryLister
}

// NewValidator creates a Validator reading registry records through cli.
func NewValidator(cli client.Client) *Validator {
	return &Validator{Client: cli}
}

// componentProperties is the subset of a type: module component's properties
// this validator can check without contacting the registry.
//
// Module and Version are decoded but not otherwise inspected. They stay on the
// struct because decoding is itself the check: a component that writes a map
// for module, or a number for version, fails to decode and is rejected.
//
// Namespace is decoded but not otherwise inspected either. It stays on the
// struct, rather than being checked only through the raw map below, because
// encoding/json's struct-field matching is case-insensitive ("Namespace",
// "NAMESPACE", ... all bind to it) while a map[string]json.RawMessage key
// lookup is exact-case only. The raw map is still needed alongside it: an
// explicit "namespace": null decodes to the same nil as the key being
// absent, so only the map's own key-presence check catches that case.
type componentProperties struct {
	Module    string  `json:"module"`
	Registry  string  `json:"registry"`
	Namespace *string `json:"namespace"`
	Version   string  `json:"version"`
}

// namespaceRemovedDetail explains why a namespace property is refused.
const namespaceRemovedDetail = "namespace is no longer accepted: a module's definitions install into the namespace of the Application that installs it"

// ValidateComponents rejects type: module components that cannot resolve for a
// reason visible locally: properties that do not decode, a namespace property,
// a registry that is not configured on this cluster, or a named registry that
// is Git backed.
//
// A component that names no registry is not checked for its source. Resolving
// which registry an empty name selects is module.ResolveRegistry's policy --
// the sole configured registry, or the one named "catalog" -- and duplicating
// that policy here is how the two would drift apart. ResolveRegistry applies it,
// and refuses Git, when the module is fetched.
func (v *Validator) ValidateComponents(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	startTime := time.Now()
	logger := logging.WithContext(ctx).WithStep("validate-module-components")
	logger.Debug("Module component validation started")

	moduleComponentCount := 0
	var errs field.ErrorList
	var registryNames []string
	var gitKinds map[string]string
	// Tracked separately from registryNames: a cluster with no registries
	// configured yields an empty list, which must still reject a named
	// registry, whereas a failed read must not reject anything -- but must not
	// be mistaken for the first case on a later component.
	registriesRead, registriesKnown := false, false

	for i, comp := range app.Spec.Components {
		if comp.Type != ComponentType {
			continue
		}
		moduleComponentCount++
		path := field.NewPath("spec", "components").Index(i).Child("properties")

		properties := componentProperties{}
		var rawProperties map[string]json.RawMessage
		if comp.Properties != nil && len(comp.Properties.Raw) > 0 {
			if err := json.Unmarshal(comp.Properties.Raw, &rawProperties); err != nil {
				// Reject rather than skip. The Application itself is malformed,
				// the render will fail later anyway, and admitting it silently
				// is the one outcome that leaves the author with no idea why.
				logger.Error(err, "Rejecting malformed module component properties", "component", comp.Name)
				errs = append(errs, field.Invalid(path, "<redacted>",
					"cannot be decoded as module component properties: "+err.Error()))
				continue
			}
			if err := json.Unmarshal(comp.Properties.Raw, &properties); err != nil {
				// Reject rather than skip. The Application itself is malformed,
				// the render will fail later anyway, and admitting it silently
				// is the one outcome that leaves the author with no idea why.
				logger.Error(err, "Rejecting malformed module component properties", "component", comp.Name)
				errs = append(errs, field.Invalid(path, "<redacted>",
					"cannot be decoded as module component properties: "+err.Error()))
				continue
			}
		}

		// namespace is no longer a supported property, so reject any explicit key
		// in any case ("Namespace", "NAMESPACE", ...), including "namespace": null.
		if _, hasNamespaceKey := rawProperties["namespace"]; hasNamespaceKey || properties.Namespace != nil {
			errs = append(errs, field.Forbidden(path.Child("namespace"), namespaceRemovedDetail))
		}

		if properties.Registry == "" {
			continue
		}
		if !registriesRead {
			names, kinds, err := v.readRegistries(ctx)
			if err != nil {
				// Fail open, and stop asking. The registry may well be fine and
				// this webhook runs with failurePolicy: Fail, so a ConfigMap
				// read that failed for its own reasons must not block the apply.
				// ResolveRegistry reports an unknown registry, and refuses a
				// Git one, at reconcile time.
				logger.Info("Skipping module registry checks", "reason", "registry-list-failed", "error", err)
			} else {
				registryNames, gitKinds, registriesKnown = names, kinds, true
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
				fmt.Sprintf("is not a configured module registry; configured: %v", registryNames)))
			continue
		}
		if kind := gitKinds[properties.Registry]; kind != "" {
			errs = append(errs, field.Invalid(path.Child("registry"), properties.Registry,
				component.GitSourceUnsupportedDetail(kind, component.ModuleGitRemedy)))
		}
	}

	logger.WithSuccess(len(errs) == 0, startTime).Debug(
		"Module component validation completed",
		"moduleComponentCount", moduleComponentCount,
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
