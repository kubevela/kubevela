/*
Copyright 2019 The Crossplane Authors.

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

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/pkg/controller/common"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/scopes/healthscope"
	"github.com/oam-dev/kubevela/pkg/controller/standard.oam.dev/v1alpha1/rollout"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
)

// Setup workload controllers.
func Setup(mgr ctrl.Manager, disableCaps string, args controller.Args) error {
	var functions []func(ctrl.Manager, controller.Args) error
	switch disableCaps {
	case common.DisableNoneCaps:
		functions = []func(ctrl.Manager, controller.Args) error{
			healthscope.Setup,
			rollout.Setup,
		}
	case common.DisableAllCaps:
	default:
		disableCapsSet := utils.StoreInSet(disableCaps)

		if !disableCapsSet.Contains(common.HealthScopeControllerName) {
			functions = append(functions, healthscope.Setup)
		}
		if !disableCapsSet.Contains(common.RolloutControllerName) {
			functions = append(functions, rollout.Setup)
		}
	}

	for _, setup := range functions {
		if err := setup(mgr, args); err != nil {
			return err
		}
	}
	return nil
}
