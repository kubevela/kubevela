/*
Copyright 2021 The KubeVela Authors.

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
	"github.com/oam-dev/kubevela/pkg/appfile"
)

// ValidateCreate validates the Application on creation
func (h *ValidatingHandler) ValidateCreate(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var componentErrs field.ErrorList
	// try to generate an app file
	appParser := appfile.NewApplicationParser(h.Client, h.dm, h.pd)

	af, err := appParser.GenerateAppFile(ctx, app)
	if err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("spec"), app, err.Error()))
		// cannot generate appfile, no need to validate further
		return componentErrs
	}
	if err := appParser.ValidateCUESchematicAppfile(af); err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("schematic"), app, err.Error()))
	}

	return componentErrs
}

// ValidateUpdate validates the Application on update
func (h *ValidatingHandler) ValidateUpdate(ctx context.Context, newApp, oldApp *v1beta1.Application) field.ErrorList {
	// check if the newApp is valid
	componentErrs := h.ValidateCreate(ctx, newApp)
	// TODO: add more validating
	return componentErrs
}
