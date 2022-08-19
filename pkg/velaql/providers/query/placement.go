/*
Copyright 2022 The KubeVela Authors.

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

package query

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

// GetPlacements get placements for application or part of its policies
func (h *provider) GetPlacements(wfctx wfContext.Context, v *value.Value, act types.Action) error {
	ctx := context.Background()
	var appName, appNamespace string
	var policyNames []string
	var err error
	if appName, err = v.GetString("appName"); err != nil {
		return err
	}
	if appNamespace, err = v.GetString("appNamespace"); err != nil {
		return err
	}
	if policyNames, err = v.GetStringSlice("policies"); err != nil {
		return err
	}
	app := &v1beta1.Application{}
	if err = h.cli.Get(ctx, client.ObjectKey{Namespace: appNamespace, Name: appName}, app); err != nil {
		return err
	}
	// TODO
	_ = policyNames
	return nil
}
