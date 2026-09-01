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

package addon

import (
	"context"
	"encoding/json"
	"time"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/logging"
)

// ComponentType is the ComponentDefinition name used by the addon-as-component
// feature: an Application component of this type installs an addon.
const ComponentType = "addon"

type compatibilityChecker func(context.Context, client.Client, *rest.Config, string, string, string) *field.Error

// Validator validates addon-specific Application components.
type Validator struct {
	// Client and RestConfig come from the manager registering this webhook.
	// They are threaded through to the compatibility checker explicitly
	// instead of read from a process-wide singleton: the webhook process
	// never initializes that singleton, so relying on it means every call
	// falls back to its lazy loader, which builds a brand-new, uncached
	// client from ambient kubeconfig state (and panics outright with none).
	Client        client.Client
	RestConfig    *rest.Config
	compatChecker compatibilityChecker
}

// NewValidator creates a Validator with the production compatibility checker,
// using cli and restConfig for registry lookups and Kubernetes-version discovery.
func NewValidator(cli client.Client, restConfig *rest.Config) *Validator {
	return &Validator{Client: cli, RestConfig: restConfig}
}

// componentProperties is the subset of a type: addon component's properties
// relevant to admission-time compatibility validation.
type componentProperties struct {
	Addon                 string `json:"addon"`
	Version               string `json:"version"`
	Registry              string `json:"registry"`
	SkipVersionValidation bool   `json:"skipVersionValidation"`
}

// ValidateComponents rejects type: addon components whose addon is incompatible
// with the current environment. It fails open on registry and lookup failures;
// only a concrete compatibility mismatch produces an error.
func (v *Validator) ValidateComponents(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	check := v.compatChecker
	if check == nil {
		check = defaultCompatChecker
	}

	startTime := time.Now()
	logger := logging.WithContext(ctx).WithStep("validate-addon-components")
	logger.Info("Addon component compatibility validation started")
	addonComponentCount := 0
	var errs field.ErrorList
	for i, component := range app.Spec.Components {
		if component.Type != ComponentType {
			continue
		}
		addonComponentCount++

		properties := componentProperties{}
		if component.Properties != nil && len(component.Properties.Raw) > 0 {
			if err := json.Unmarshal(component.Properties.Raw, &properties); err != nil {
				logger.Debug("Skipping malformed addon component properties",
					"component", component.Name, "error", err)
				continue
			}
		}
		if properties.SkipVersionValidation {
			logger.Debug("Skipping addon compatibility validation",
				"component", component.Name, "reason", "version-validation-disabled")
			continue
		}

		addonName := properties.Addon
		if addonName == "" {
			addonName = component.Name
		}
		if compatibilityErr := check(ctx, v.Client, v.RestConfig, addonName, properties.Version, properties.Registry); compatibilityErr != nil {
			errs = append(errs, field.Invalid(
				field.NewPath("spec", "components").Index(i).Child("properties"),
				addonName,
				compatibilityErr.Detail,
			))
		}
	}

	logger.WithSuccess(len(errs) == 0, startTime).Info(
		"Addon component compatibility validation completed",
		"addonComponentCount", addonComponentCount,
		"errorCount", len(errs),
	)
	return errs
}
