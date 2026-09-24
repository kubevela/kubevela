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

// registryGitKindLister reports which configured module registries are Git
// backed, keyed by name. A seam for tests; production uses
// listClusterRegistryGitKinds.
type registryGitKindLister func(context.Context, client.Client) (map[string]string, error)

// listClusterRegistryGitKinds is the production registryGitKindLister. It reads
// the module registry ConfigMap and no token Secret.
//
// Only the Git kinds are asked for. An unknown registry name is
// module.ResolveRegistry's to report, with the list of what is configured, and
// duplicating that here would give the author two different messages for one
// mistake -- so the sorted name list is not built.
func listClusterRegistryGitKinds(ctx context.Context, cli client.Client) (map[string]string, error) {
	return component.ListRegistryGitKindsIn(ctx, cli, pkgmodule.ModuleRegistryConfigMap)
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

	listRegistryGitKinds registryGitKindLister
}

// NewValidator creates a Validator reading registry records through cli.
func NewValidator(cli client.Client) *Validator {
	return &Validator{Client: cli}
}

// componentProperties is the subset of a type: module component's properties
// this validator can check without contacting the registry.
//
// Module, Namespace and Version are decoded but not otherwise inspected. They
// stay on the struct because decoding is itself the check: a component that
// writes a map for module, or a number for version, fails to decode and is
// rejected.
type componentProperties struct {
	Module    string `json:"module"`
	Registry  string `json:"registry"`
	Namespace string `json:"namespace"`
	Version   string `json:"version"`
}

// ValidateComponents rejects type: module components that cannot resolve for a
// reason visible locally: properties that do not decode, or a named registry
// that is Git backed.
//
// A component that names no registry is not checked for its source. Resolving
// which registry an empty name selects is module.ResolveRegistry's policy --
// the sole configured registry, or the one named "catalog" -- and duplicating
// that policy here is how the two would drift apart. ResolveRegistry applies it,
// and refuses Git, when the module is fetched.
//
// Unknown registry names are likewise left to ResolveRegistry, which already
// reports them with the list of what is configured.
func (v *Validator) ValidateComponents(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	startTime := time.Now()
	logger := logging.WithContext(ctx).WithStep("validate-module-components")
	logger.Debug("Module component validation started")

	moduleComponentCount := 0
	var errs field.ErrorList
	var gitKinds map[string]string
	// Tracked separately from gitKinds: a cluster with no Git registry yields an
	// empty map, which must reject nothing, whereas a failed read must also
	// reject nothing -- but must not be mistaken for the first case on a later
	// component.
	kindsRead, kindsKnown := false, false

	for i, comp := range app.Spec.Components {
		if comp.Type != ComponentType {
			continue
		}
		moduleComponentCount++
		path := field.NewPath("spec", "components").Index(i).Child("properties")

		properties := componentProperties{}
		if comp.Properties != nil && len(comp.Properties.Raw) > 0 {
			if err := json.Unmarshal(comp.Properties.Raw, &properties); err != nil {
				// Reject rather than skip. The Application itself is malformed,
				// the render will fail later anyway, and admitting it silently
				// is the one outcome that leaves the author with no idea why.
				logger.Error(err, "Rejecting malformed module component properties", "component", comp.Name)
				errs = append(errs, field.Invalid(path, string(comp.Properties.Raw),
					"cannot be decoded as module component properties: "+err.Error()))
				continue
			}
		}

		if properties.Registry == "" {
			continue
		}
		if !kindsRead {
			kinds, err := v.readRegistryGitKinds(ctx)
			if err != nil {
				// Fail open, and stop asking. The registry may well be fine and
				// this webhook runs with failurePolicy: Fail, so a ConfigMap
				// read that failed for its own reasons must not block the apply.
				// ResolveRegistry refuses a Git registry at reconcile time.
				logger.Info("Skipping module registry source check", "reason", "registry-list-failed", "error", err)
			} else {
				gitKinds, kindsKnown = kinds, true
			}
			kindsRead = true
		}
		if !kindsKnown {
			continue
		}
		if kind := gitKinds[properties.Registry]; kind != "" {
			errs = append(errs, field.Invalid(path.Child("registry"), properties.Registry,
				component.GitSourceUnsupportedDetail(kind, component.ModuleGitRemedy)))
		}
	}

	logger.WithSuccess(len(errs) == 0, startTime).Info(
		"Module component validation completed",
		"moduleComponentCount", moduleComponentCount,
		"errorCount", len(errs),
	)
	return errs
}

func (v *Validator) readRegistryGitKinds(ctx context.Context) (map[string]string, error) {
	list := v.listRegistryGitKinds
	if list == nil {
		list = listClusterRegistryGitKinds
	}
	return list(ctx, v.Client)
}
