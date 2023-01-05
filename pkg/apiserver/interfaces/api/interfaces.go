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

// viewPrefix the path prefix for view page
var viewPrefix = "/view"

// Interface the API should define the http route
type Interface interface {
	GetWebServiceRoute() *restful.WebService
}

var registeredAPI []Interface

// RegisterAPI register API handler
func RegisterAPI(ws Interface) {
	registeredAPI = append(registeredAPI, ws)
}

// GetRegisteredAPI return all API handlers
func GetRegisteredAPI() []Interface {
	return registeredAPI
}

func returns200(b *restful.RouteBuilder) {
	b.Returns(http.StatusOK, "OK", apisv1.SimpleResponse{Status: "ok"})
}

func returns500(b *restful.RouteBuilder) {
	b.Returns(http.StatusInternalServerError, "Bummer, something went wrong", nil)
}

// InitAPIBean inits all API handlers, pass in the required parameter object.
// It can be implemented using the idea of dependency injection.
func InitAPIBean() []interface{} {
	// Application
	RegisterAPI(NewApplication())
	RegisterAPI(NewProject())
	RegisterAPI(NewEnv())
	RegisterAPI(NewPipeline())

	// Extension
	RegisterAPI(NewDefinition())
	RegisterAPI(NewAddon())
	RegisterAPI(NewEnabledAddon())
	RegisterAPI(NewAddonRegistry())

	// Config management
	RegisterAPI(Config())
	RegisterAPI(ConfigTemplate())

	// Resources
	RegisterAPI(NewCluster())
	RegisterAPI(NewOAMApplication())
	RegisterAPI(NewPayloadTypes())
	RegisterAPI(NewTarget())
	RegisterAPI(NewVelaQL())
	RegisterAPI(NewWebhook())
	RegisterAPI(NewRepository())
	RegisterAPI(NewCloudShell())

	// Authentication
	RegisterAPI(NewAuthentication())
	RegisterAPI(NewUser())
	RegisterAPI(NewSystemInfo())
	RegisterAPI(NewCloudShellView())

	// RBAC
	RegisterAPI(NewRBAC())
	var beans []interface{}
	for i := range registeredAPI {
		beans = append(beans, registeredAPI[i])
	}
	beans = append(beans, NewWorkflow())
	return beans
}
