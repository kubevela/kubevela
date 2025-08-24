package app

import (
	"fmt"

	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/application"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/componentdefinition"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/core"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/policydefinition"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1beta1/traitdefinition"
	"github.com/oam-dev/kubevela/pkg/webhook/definition"
	"github.com/oam-dev/kubevela/pkg/webhook/multicluster"
	ctrl "sigs.k8s.io/controller-runtime"
)

// RegisterWebhooks register all webhooks.
func RegisterWebhooks(mgr ctrl.Manager) error {
	for i, webhookSetup := range []func(ctrl.Manager) error{
		multicluster.RegisterWebhooks,
		core.RegisterWebhooks,
		application.RegisterWebhooks,
		componentdefinition.RegisterWebhooks,
		traitdefinition.RegisterWebhooks,
		policydefinition.RegisterWebhooks,
		definition.SetupWebhook,
	} {
		if err := webhookSetup(mgr); err != nil {
			return fmt.Errorf("failed to register webhooks at index %d: %w", i, err)
		}
	}
	return nil
}
