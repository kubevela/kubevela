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

package v1alpha2

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationdeployment"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/scopes/healthscope"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/traits/manualscalertrait"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/workloads/containerizedworkload"
)

// Setup workload controllers.
func Setup(mgr ctrl.Manager, args controller.Args, l logging.Logger) error {
	for _, setup := range []func(ctrl.Manager, controller.Args, logging.Logger) error{
		applicationconfiguration.Setup,
		containerizedworkload.Setup, manualscalertrait.Setup, healthscope.Setup,
		applicationdeployment.Setup, application.Setup,
	} {
		if err := setup(mgr, args, l); err != nil {
			return err
		}
	}
	return nil
}
