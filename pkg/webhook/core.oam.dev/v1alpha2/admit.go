package v1alpha2

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/component"
)

// Add will be called in main and register all validation handlers
func Add(mgr manager.Manager) error {
	application.RegisterValidatingHandler(mgr)

	if err := applicationconfiguration.RegisterValidatingHandler(mgr); err != nil {
		return err
	}
	applicationconfiguration.RegisterMutatingHandler(mgr)
	if err := component.RegisterMutatingHandler(mgr); err != nil {
		return err
	}
	component.RegisterValidatingHandler(mgr)
	return nil
}
