/*
 Copyright 2026. The KubeVela Authors.

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

// Package v2alpha1 aggregates the core.oam.dev/v2alpha1 controllers, mirroring
// the shape of pkg/controller/core.oam.dev/v1beta1/setup.go. It currently
// has exactly one member: the Operations KEP (KEP 2.15) implementation.
package v2alpha1

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v2alpha1/operation"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

// Setup workload controllers.
func Setup(mgr ctrl.Manager, args controller.Args) error {
	for _, setup := range []func(ctrl.Manager, controller.Args) error{
		operation.Setup,
	} {
		if err := setup(mgr, args); err != nil {
			return err
		}
	}
	return nil
}
