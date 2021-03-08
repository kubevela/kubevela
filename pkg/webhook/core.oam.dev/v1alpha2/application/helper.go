package application

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

const (
	errFmtTraitNameConflict = "traits name (%q) conflict between of component %q"
)

// ValidatingApp is used for validating Application
type ValidatingApp struct {
	app *v1alpha2.Application
}

// AppValidator provides functions to validate Application
type AppValidator interface {
	Validate(context.Context, ValidatingApp) []error
}

// AppValidateFunc implements function to validate Application
type AppValidateFunc func(context.Context, ValidatingApp) []error

// Validate validates Application
func (fn AppValidateFunc) Validate(ctx context.Context, v ValidatingApp) []error {
	return fn(ctx, v)
}

// ValidateTraitNameFn validates traitName should be unique in application
func ValidateTraitNameFn(_ context.Context, v ValidatingApp) []error {
	klog.Info("validate trait name conflicts ", "app name:", v.app.Name)
	allErrs := make([]error, 0)
	for _, component := range v.app.Spec.Components {
		allTraitName := make(map[string]bool)
		for _, trait := range component.Traits {
			if _, ok := allTraitName[trait.Name]; ok {
				allErrs = append(allErrs, field.Invalid(field.NewPath("spec"), v.app.Spec,
					fmt.Sprintf(errFmtTraitNameConflict, trait.Name, component.Name)))
			} else {
				allTraitName[trait.Name] = true
			}
		}
	}
	return allErrs
}
