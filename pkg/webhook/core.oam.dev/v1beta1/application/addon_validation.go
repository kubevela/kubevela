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
	"slices"

	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/logging"
	addonvalidation "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/application/addon"
)

// componentValidator is the shape of a per-component-type admission check.
// The addon and module checks share it: both answer from the Application plus
// a registry ConfigMap, and both return field errors against the component's
// properties path.
type componentValidator interface {
	ValidateComponents(context.Context, *v1beta1.Application) field.ErrorList
}

// ValidateAddonComponents validates addon components when the feature is enabled.
func (h *ValidatingHandler) ValidateAddonComponents(
	ctx context.Context,
	app *v1beta1.Application,
) field.ErrorList {
	return h.validateComponentsOfType(ctx, app, componentCheck{
		gate:          features.EnableAddonComponent,
		componentType: addonvalidation.ComponentType,
		step:          "validate-addon-components",
		validator:     h.addonValidator,
		newValidator:  func() componentValidator { return addonvalidation.NewValidator(h.Client) },
	})
}

// componentCheck is one component type's admission check: the gate that turns
// it on, the type it applies to, its log step, and the validator to run.
type componentCheck struct {
	gate          featuregate.Feature
	componentType string
	step          string
	// validator is the handler's injected validator, nil outside tests.
	validator    componentValidator
	newValidator func() componentValidator
}

// validateComponentsOfType runs one componentCheck. The addon and module
// checks differ only in the struct above, so the gate test, the
// does-this-Application-even-have-one scan and the validator fallback live
// here once rather than in each caller.
//
// The logger is built after both short-circuits, not before: WithStep copies a
// key-value slice into a new sink, and this runs on every Application
// admission including the controller's own metadata writes, so the disabled
// and not-applicable paths stay allocation free.
func (h *ValidatingHandler) validateComponentsOfType(
	ctx context.Context,
	app *v1beta1.Application,
	check componentCheck,
) field.ErrorList {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(check.gate) {
		return nil
	}
	if !slices.ContainsFunc(app.Spec.Components, func(component common.ApplicationComponent) bool {
		return component.Type == check.componentType
	}) {
		return nil
	}
	logging.WithContext(ctx).WithStep(check.step).Debug(
		"Validating components", "componentType", check.componentType)
	validator := check.validator
	if validator == nil {
		validator = check.newValidator()
	}
	return validator.ValidateComponents(ctx, app)
}
