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

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/appdeployment"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationrollout"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/components/componentdefinition"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/policies/policydefinition"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/traits/traitdefinition"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core/workflow/workflowstepdefinition"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/initializer"
)

// Setup workload controllers.
func Setup(mgr ctrl.Manager, args controller.Args) error {
	if args.OAMSpecVer == "v0.3" || args.OAMSpecVer == "all" {
		for _, setup := range []func(ctrl.Manager, controller.Args) error{
			application.Setup, applicationrollout.Setup, appdeployment.Setup,
			traitdefinition.Setup, componentdefinition.Setup, policydefinition.Setup, workflowstepdefinition.Setup,
			initializer.Setup,
		} {
			if err := setup(mgr, args); err != nil {
				return err
			}
		}
	}
	if args.OAMSpecVer == "v0.2" || args.OAMSpecVer == "all" {
		if err := applicationconfiguration.Setup(mgr, args); err != nil {
			return err
		}
	}
	return nil
}
