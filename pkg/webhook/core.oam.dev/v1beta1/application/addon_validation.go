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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/logging"
	addonvalidation "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/application/addon"
)

type addonComponentValidator interface {
	ValidateComponents(context.Context, *v1beta1.Application) field.ErrorList
}

// ValidateAddonComponents validates addon components when the feature is enabled.
func (h *ValidatingHandler) ValidateAddonComponents(
	ctx context.Context,
	app *v1beta1.Application,
) field.ErrorList {
	logger := logging.WithContext(ctx).WithStep("validate-addon-components")
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableAddonComponent) {
		logger.Debug("Skipping addon component validation", "reason", "feature-gate-disabled")
		return nil
	}
	if !slices.ContainsFunc(app.Spec.Components, func(component common.ApplicationComponent) bool {
		return component.Type == addonvalidation.ComponentType
	}) {
		logger.Debug("Skipping addon component validation", "reason", "no-addon-component")
		return nil
	}
	validator := h.addonValidator
	if validator == nil {
		validator = addonvalidation.NewValidator()
	}
	return validator.ValidateComponents(ctx, app)
}
