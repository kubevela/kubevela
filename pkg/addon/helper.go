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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// EnableAddon will enable addon with dependency check, source is where addon from.
func EnableAddon(ctx context.Context, addon *Addon, cli client.Client, apply apply.Applicator, config *rest.Config, source Source, args map[string]interface{}) error {
	h := newAddonHandler(ctx, addon, cli, apply, config, source, args)
	err := h.enableAddon(ctx, cli)
	if err != nil {
		return err
	}
	return nil
}

// DisableAddon will disable addon from cluster.
func DisableAddon(ctx context.Context, cli client.Client, name string) error {
	app := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      Convert2AppName(name),
			Namespace: types.DefaultKubeVelaNS,
		},
	}
	err := cli.Delete(ctx, app)
	if err != nil {
		return err
	}
	return nil
}
