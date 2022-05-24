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

package api

import (
	"net/http"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"

	"github.com/emicklei/go-restful/v3"
)

// versionPrefix API version prefix.
var versionPrefix = "/api/v1"

// Interface the API should define the http route
type Interface interface {
	GetWebServiceRoute() *restful.WebService
}

var registeredAPIInterface []Interface

// RegisterAPIInterface register APIInterface
func RegisterAPIInterface(ws Interface) {
	registeredAPIInterface = append(registeredAPIInterface, ws)
}

// GetRegisteredAPIInterface return registeredAPIInterface
func GetRegisteredAPIInterface() []Interface {
	return registeredAPIInterface
}

func returns200(b *restful.RouteBuilder) {
	b.Returns(http.StatusOK, "OK", apisv1.SimpleResponse{Status: "ok"})
}

func returns500(b *restful.RouteBuilder) {
	b.Returns(http.StatusInternalServerError, "Bummer, something went wrong", nil)
}

// InitAPIBean inits all APIInterface, pass in the required parameter object.
// It can be implemented using the idea of dependency injection.
func InitAPIBean() []interface{} {
	// Application
	RegisterAPIInterface(NewApplicationAPIInterface())
	RegisterAPIInterface(NewProjectAPIInterface())
	RegisterAPIInterface(NewEnvAPIInterface())

	// Extension
	RegisterAPIInterface(NewDefinitionAPIInterface())
	RegisterAPIInterface(NewAddonAPIInterface())
	RegisterAPIInterface(NewEnabledAddonAPIInterface())
	RegisterAPIInterface(NewAddonRegistryAPIInterface())

	// Config management
	RegisterAPIInterface(ConfigAPIInterface())

	// Resources
	RegisterAPIInterface(NewClusterAPIInterface())
	RegisterAPIInterface(NewOAMApplication())
	RegisterAPIInterface(NewPayloadTypesAPIInterface())
	RegisterAPIInterface(NewTargetAPIInterface())
	RegisterAPIInterface(NewVelaQLAPIInterface())
	RegisterAPIInterface(NewWebhookAPIInterface())
	RegisterAPIInterface(NewHelmAPIInterface())

	// Authentication
	RegisterAPIInterface(NewAuthenticationAPIInterface())
	RegisterAPIInterface(NewUserAPIInterface())
	RegisterAPIInterface(NewSystemInfoAPIInterface())

	// RBAC
	RegisterAPIInterface(NewRBACAPIInterface())
	var beans []interface{}
	for i := range registeredAPIInterface {
		beans = append(beans, registeredAPIInterface[i])
	}
	beans = append(beans, NewWorkflowAPIInterface())
	return beans
}
