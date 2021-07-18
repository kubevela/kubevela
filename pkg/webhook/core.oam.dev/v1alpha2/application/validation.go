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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/webhook/common/rollout"
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
	if i, err := appParser.ValidateComponentNames(ctx, af); err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].name", i)), app, err.Error()))
	}
	if err := appParser.ValidateCUESchematicAppfile(af); err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("schematic"), app, err.Error()))
	}
	if v := app.GetAnnotations()[oam.AnnotationAppRollout]; len(v) != 0 && v != "true" {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("annotation:app.oam.dev/rollout-template"), app, "the annotation value of rollout-template must be true"))
	}
	if app.Spec.RolloutPlan != nil {
		componentErrs = append(componentErrs, rollout.ValidateCreate(h.Client, app.Spec.RolloutPlan, field.NewPath("rolloutPlan"))...)
	}
	componentErrs = append(componentErrs, h.validateExternalRevisionName(ctx, app)...)
	return componentErrs
}

// ValidateUpdate validates the Application on update
func (h *ValidatingHandler) ValidateUpdate(ctx context.Context, newApp, oldApp *v1beta1.Application) field.ErrorList {
	// check if the newApp is valid
	componentErrs := h.ValidateCreate(ctx, newApp)
	// TODO: add more validating
	return componentErrs
}

func (h *ValidatingHandler) validateExternalRevisionName(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var componentErrs field.ErrorList

	for index, comp := range app.Spec.Components {
		if len(comp.ExternalRevision) == 0 {
			continue
		}

		revisionName := comp.ExternalRevision
		cr := &appsv1.ControllerRevision{}
		if err := h.Client.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: revisionName}, cr); err != nil {
			if !apierrors.IsNotFound(err) {
				componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, err.Error()))
			}
			continue
		}

		labeledControllerComponent := cr.GetLabels()[oam.LabelControllerRevisionComponent]
		labeledRevisionHash := cr.GetLabels()[oam.LabelComponentRevisionHash]
		if labeledControllerComponent != comp.Name {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, fmt.Sprintf("label:%s for revision:%s should be equal with component name", oam.LabelControllerRevisionComponent, revisionName)))
		}
		if len(labeledRevisionHash) == 0 {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, fmt.Sprintf("label:%s for revision:%s should exist", oam.LabelComponentRevisionHash, revisionName)))
			continue
		}

		existingComp, err := util.RawExtension2Component(cr.Data)
		if err != nil {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, "cann't covert revision to component"))
			continue
		}
		// recalculate hash
		hash, err := utils.ComputeSpecHash(existingComp)
		if err != nil {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, "cann't recalculate hash"))
			continue
		}
		if hash != labeledRevisionHash {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, "hash value which specified in labels and revision data's hash should be equal"))
			continue
		}
	}
	return componentErrs
}
