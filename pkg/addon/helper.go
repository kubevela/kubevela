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

package addon

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// disabled indicates the addon is disabled
	disabled = "disabled"
	// enabled indicates the addon is enabled
	enabled = "enabled"
	// enabling indicates the addon is enabling
	enabling = "enabling"
	// disabling indicates the addon related app is deleting
	disabling = "disabling"
	// suspend indicates the addon related app is suspend
	suspend = "suspend"
)

// EnableAddon will enable addon with dependency check, source is where addon from.
func EnableAddon(ctx context.Context, addon *Addon, cli client.Client, apply apply.Applicator, config *rest.Config, source Source, args map[string]interface{}) error {
	h := newAddonHandler(ctx, addon, cli, apply, config, source, args)
	err := h.enableAddon()
	if err != nil {
		return err
	}
	return nil
}

// DisableAddon will disable addon from cluster.
func DisableAddon(ctx context.Context, cli client.Client, name string) error {
	app, err := FetchAddonRelatedApp(ctx, cli, name)
	// if app not exist, report error
	if err != nil {
		return err
	}
	if err := cli.Delete(ctx, app); err != nil {
		return err
	}
	return nil
}

// GetAddonStatus is genrall func for cli and apiServer get addon status
func GetAddonStatus(ctx context.Context, cli client.Client, name string) (Status, error) {
	app, err := FetchAddonRelatedApp(ctx, cli, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Status{AddonPhase: disabled, AppStatus: nil}, nil
		}
		return Status{}, err
	}
	if app.Status.Workflow != nil && app.Status.Workflow.Suspend {
		return Status{AddonPhase: suspend, AppStatus: &app.Status}, nil
	}
	switch app.Status.Phase {
	case commontypes.ApplicationRunning:
		return Status{AddonPhase: enabled, AppStatus: &app.Status}, nil
	case commontypes.ApplicationDeleting:
		return Status{AddonPhase: disabling, AppStatus: &app.Status}, nil
	default:
		return Status{AddonPhase: enabling, AppStatus: &app.Status}, nil
	}
}

// Status contain addon phase and related app status
type Status struct {
	AddonPhase string
	AppStatus  *commontypes.AppStatus
}
