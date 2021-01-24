package core_oam_dev

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationdeployment"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/component"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/traitdefinition"
)

// Register will be called in main and register all validation handlers
func Register(mgr manager.Manager) error {
	if err := application.RegisterValidatingHandler(mgr); err != nil {
		return err
	}
	if err := applicationconfiguration.RegisterValidatingHandler(mgr); err != nil {
		return err
	}
	if err := traitdefinition.RegisterValidatingHandler(mgr); err != nil {
		return err
	}
	applicationconfiguration.RegisterMutatingHandler(mgr)
	applicationdeployment.RegisterMutatingHandler(mgr)
	if err := component.RegisterMutatingHandler(mgr); err != nil {
		return err
	}
	component.RegisterValidatingHandler(mgr)
	return nil
}
