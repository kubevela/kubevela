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

package v1beta1

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core/components/componentdefinition"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core/policies/policydefinition"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core/traits/traitdefinition"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core/workflow/workflowstepdefinition"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

// Setup workload controllers.
func Setup(mgr ctrl.Manager, args controller.Args) error {
	for _, setup := range []func(ctrl.Manager, controller.Args) error{
		application.Setup, traitdefinition.Setup, componentdefinition.Setup, policydefinition.Setup, workflowstepdefinition.Setup,
	} {
		if err := setup(mgr, args); err != nil {
			return err
		}
	}
	return nil
}
