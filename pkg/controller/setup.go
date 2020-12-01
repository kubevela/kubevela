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

	autoscalers "github.com/oam-dev/kubevela/pkg/controller/standard.oam.dev/v1alpha1/autoscaler"
	"github.com/oam-dev/kubevela/pkg/controller/standard.oam.dev/v1alpha1/metrics"
	"github.com/oam-dev/kubevela/pkg/controller/standard.oam.dev/v1alpha1/podspecworkload"
	"github.com/oam-dev/kubevela/pkg/controller/standard.oam.dev/v1alpha1/routes"
)

// Setup workload controllers.
func Setup(mgr ctrl.Manager, capabilities []string) error {
	for _, cap := range capabilities {
		var setup func(ctrl.Manager) error
		switch cap {
		case "metrics":
			setup = metrics.Setup
		case "podspecworkload":
			setup = podspecworkload.Setup
		case "route":
			setup = routes.Setup
		case "autoscale":
			setup = autoscalers.Setup
		}
		if setup != nil {
			if err := setup(mgr); err != nil {
				return err
			}
		}
	}
	return nil
}
