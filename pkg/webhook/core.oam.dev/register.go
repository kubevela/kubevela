package core_oam_dev

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationdeployment"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/component"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/traitdefinition"
)

// Register will be called in main and register all validation handlers
func Register(mgr manager.Manager, args controller.Args) {
	application.RegisterValidatingHandler(mgr, args)
	applicationconfiguration.RegisterValidatingHandler(mgr, args)
	traitdefinition.RegisterValidatingHandler(mgr, args)
	applicationconfiguration.RegisterMutatingHandler(mgr)
	applicationdeployment.RegisterMutatingHandler(mgr)
	applicationdeployment.RegisterValidatingHandler(mgr)
	component.RegisterMutatingHandler(mgr, args)
	component.RegisterValidatingHandler(mgr)

	server := mgr.GetWebhookServer()
	server.Register("/convert", &conversion.Webhook{})
}
