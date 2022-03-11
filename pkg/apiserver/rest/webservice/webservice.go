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

package webservice

import (
	"net/http"
	"time"

	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
)

// versionPrefix API version prefix.
var versionPrefix = "/api/v1"

// WebService webservice interface
type WebService interface {
	GetWebService() *restful.WebService
}

var registeredWebService []WebService

// RegisterWebService regist webservice
func RegisterWebService(ws WebService) {
	registeredWebService = append(registeredWebService, ws)
}

// GetRegisteredWebService return registeredWebService
func GetRegisteredWebService() []WebService {
	return registeredWebService
}

func noop(req *restful.Request, resp *restful.Response) {}

func returns200(b *restful.RouteBuilder) {
	b.Returns(http.StatusOK, "OK", apisv1.SimpleResponse{Status: "ok"})
}

func returns500(b *restful.RouteBuilder) {
	b.Returns(http.StatusInternalServerError, "Bummer, something went wrong", nil)
}

// Init init all webservice, pass in the required parameter object.
// It can be implemented using the idea of dependency injection.
func Init(ds datastore.DataStore, addonCacheTime time.Duration) {
	clusterUsecase := usecase.NewClusterUsecase(ds)
	envUsecase := usecase.NewEnvUsecase(ds)
	workflowUsecase := usecase.NewWorkflowUsecase(ds, envUsecase)
	projectUsecase := usecase.NewProjectUsecase(ds)
	targetUsecase := usecase.NewTargetUsecase(ds)
	oamApplicationUsecase := usecase.NewOAMApplicationUsecase()
	velaQLUsecase := usecase.NewVelaQLUsecase()
	definitionUsecase := usecase.NewDefinitionUsecase()
	addonUsecase := usecase.NewAddonUsecase(addonCacheTime)
	envBindingUsecase := usecase.NewEnvBindingUsecase(ds, workflowUsecase, definitionUsecase, envUsecase)
	applicationUsecase := usecase.NewApplicationUsecase(ds, workflowUsecase, envBindingUsecase, envUsecase, targetUsecase, definitionUsecase, projectUsecase)
	webhookUsecase := usecase.NewWebhookUsecase(ds, applicationUsecase)
	systemInfoUsecase := usecase.NewSystemInfoUsecase(ds)
	helmUsecase := usecase.NewHelmUsecase()
	authenticationUsecase := usecase.NewAuthenticationUsecase(ds)

	// init for default values

	// Application
	RegisterWebService(NewApplicationWebService(applicationUsecase, envBindingUsecase, workflowUsecase))
	RegisterWebService(NewProjectWebService(projectUsecase))
	RegisterWebService(NewEnvWebService(envUsecase, applicationUsecase))

	// Extension
	RegisterWebService(NewDefinitionWebservice(definitionUsecase))
	RegisterWebService(NewAddonWebService(addonUsecase))
	RegisterWebService(NewEnabledAddonWebService(addonUsecase))
	RegisterWebService(NewAddonRegistryWebService(addonUsecase))

	// Resources
	RegisterWebService(NewClusterWebService(clusterUsecase))
	RegisterWebService(NewOAMApplication(oamApplicationUsecase))
	RegisterWebService(&policyDefinitionWebservice{})
	RegisterWebService(&payloadTypesWebservice{})
	RegisterWebService(NewTargetWebService(targetUsecase, applicationUsecase))
	RegisterWebService(NewVelaQLWebService(velaQLUsecase))
	RegisterWebService(NewWebhookWebService(webhookUsecase, applicationUsecase))

	// Authentication
	RegisterWebService(NewAuthenticationWebService(authenticationUsecase))

	RegisterWebService(NewSystemInfoWebService(systemInfoUsecase))

	RegisterWebService(NewHelmWebService(helmUsecase))
}
