package application

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ValidateCreate validates the Application on creation
func (h *ValidatingHandler) ValidateCreate(app *v1alpha2.Application) field.ErrorList {
	var componentErrs field.ErrorList
	// try to generate an app file
	appParser := application.NewApplicationParser(h.Client, h.dm)
	if _, err := appParser.GenerateAppFile(app.Name, app); err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("spec"), app, err.Error()))
	}
	return componentErrs
}

// ValidateUpdate validates the Application on update
func (h *ValidatingHandler) ValidateUpdate(newApp, oldApp *v1alpha2.Application) field.ErrorList {
	// check if the newApp is valid
	componentErrs := h.ValidateCreate(newApp)
	// one can't add a rollout annotation to an existing application
	if _, exist := oldApp.GetAnnotations()[oam.AnnotationAppRollout]; !exist {
		if _, exist := newApp.GetAnnotations()[oam.AnnotationAppRollout]; exist {
			componentErrs = append(componentErrs, field.Forbidden(field.NewPath("meta").Child("annotation"),
				"cannot add a rollout annotation on an existing application"))
		}
	}
	return componentErrs
}
