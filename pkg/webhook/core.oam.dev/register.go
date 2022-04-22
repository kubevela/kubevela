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

package core_oam_dev

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/component"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/componentdefinition"
	"github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/traitdefinition"
)

// Register will be called in main and register all validation handlers
func Register(mgr manager.Manager, args controller.Args) {
	switch args.OAMSpecVer {
	case "all":
		application.RegisterValidatingHandler(mgr, args)
		componentdefinition.RegisterMutatingHandler(mgr, args)
		componentdefinition.RegisterValidatingHandler(mgr, args)
		traitdefinition.RegisterValidatingHandler(mgr, args)
		applicationconfiguration.RegisterMutatingHandler(mgr)
		applicationconfiguration.RegisterValidatingHandler(mgr, args)
		component.RegisterMutatingHandler(mgr, args)
		component.RegisterValidatingHandler(mgr)
	case "minimal":
		application.RegisterValidatingHandler(mgr, args)
		componentdefinition.RegisterMutatingHandler(mgr, args)
		componentdefinition.RegisterValidatingHandler(mgr, args)
		traitdefinition.RegisterValidatingHandler(mgr, args)
	case "v0.3":
		application.RegisterValidatingHandler(mgr, args)
		application.RegisterMutatingHandler(mgr)
		componentdefinition.RegisterMutatingHandler(mgr, args)
		componentdefinition.RegisterValidatingHandler(mgr, args)
		traitdefinition.RegisterValidatingHandler(mgr, args)
	case "v0.2":
		applicationconfiguration.RegisterMutatingHandler(mgr)
		applicationconfiguration.RegisterValidatingHandler(mgr, args)
		component.RegisterMutatingHandler(mgr, args)
		component.RegisterValidatingHandler(mgr)
	}

	server := mgr.GetWebhookServer()
	server.Register("/convert", &conversion.Webhook{})
}
