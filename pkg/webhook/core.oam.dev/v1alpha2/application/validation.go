package application

import (
	"context"
	"fmt"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile"
)

// ValidateCreate validates the Application on creation
func (h *ValidatingHandler) ValidateCreate(ctx context.Context, app *v1alpha2.Application) field.ErrorList {
	var componentErrs field.ErrorList
	// try to generate an app file
	appParser := appfile.NewApplicationParser(h.Client, h.dm)
	if _, err := appParser.GenerateAppFile(ctx, app.Name, app); err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("spec"), app, err.Error()))
		return componentErrs
	}
	vApp := &ValidatingApp{app: app}
	for _, validator := range h.Validators {
		if allErrs := validator.Validate(ctx, *vApp); len(allErrs) != 0 {
			// utilerrors.NewAggregate can remove nil from allErrs
			klog.InfoS("validation failed", " name: ", app.Name, " errMsgi: ",
				utilerrors.NewAggregate(allErrs).Error())
			for _, err := range allErrs {
				componentErrs = append(componentErrs, field.Invalid(field.NewPath("spec"), app.Spec,
					fmt.Sprintf("validation failed, err = %s", err.Error())))
			}
		}
	}
	return componentErrs
}

// ValidateUpdate validates the Application on update
func (h *ValidatingHandler) ValidateUpdate(ctx context.Context, newApp, oldApp *v1alpha2.Application) field.ErrorList {
	// check if the newApp is valid
	componentErrs := h.ValidateCreate(ctx, newApp)
	// TODO: add more validating
	return componentErrs
}
