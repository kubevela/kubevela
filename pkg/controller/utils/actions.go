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

package utils

import (
	"context"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// FreezeApplication freeze application to disable the reconciling process for it
func FreezeApplication(ctx context.Context, cli client.Client, app *v1beta1.Application, mutate func()) (string, error) {
	return oam.GetControllerRequirement(app), _updateApplicationWithControllerRequirement(ctx, cli, app, mutate, "Disabled")
}

// UnfreezeApplication unfreeze application to enable the reconciling process for it
func UnfreezeApplication(ctx context.Context, cli client.Client, app *v1beta1.Application, mutate func(), originalControllerRequirement string) error {
	return _updateApplicationWithControllerRequirement(ctx, cli, app, mutate, originalControllerRequirement)
}

func _updateApplicationWithControllerRequirement(ctx context.Context, cli client.Client, app *v1beta1.Application, mutate func(), controllerRequirement string) error {
	appKey := client.ObjectKeyFromObject(app)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := cli.Get(ctx, appKey, app); err != nil {
			return err
		}
		oam.SetControllerRequirement(app, controllerRequirement)
		if mutate != nil {
			mutate()
		}
		return cli.Update(ctx, app)
	})
}
