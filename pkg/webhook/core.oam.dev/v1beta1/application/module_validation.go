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

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	modulevalidation "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/application/module"
)

// ValidateModuleComponents validates module components when the feature is enabled.
func (h *ValidatingHandler) ValidateModuleComponents(
	ctx context.Context,
	app *v1beta1.Application,
) field.ErrorList {
	return h.validateComponentsOfType(ctx, app, componentCheck{
		gate:          features.EnableModuleComponent,
		componentType: modulevalidation.ComponentType,
		step:          "validate-module-components",
		validator:     h.moduleValidator,
		newValidator:  func() componentValidator { return modulevalidation.NewValidator(h.Client) },
	})
}
