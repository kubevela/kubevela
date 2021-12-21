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
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
)

// versionPrefix API version prefix.
var versionPrefix = "/api/v1"

// WebService webservice interface
type WebService interface {
	GetWebService() *restful.WebService
}

var registedWebService []WebService

// RegistWebService regist webservice
func RegistWebService(ws WebService) {
	registedWebService = append(registedWebService, ws)
}

// GetRegistedWebService return registedWebService
func GetRegistedWebService() []WebService {
	return registedWebService
}

func noop(req *restful.Request, resp *restful.Response) {}

func returns200(b *restful.RouteBuilder) {
	b.Returns(http.StatusOK, "OK", map[string]string{"status": "ok"})
}

func returns500(b *restful.RouteBuilder) {
	b.Returns(http.StatusInternalServerError, "Bummer, something went wrong", nil)
}

// Init init all webservice, pass in the required parameter object.
// It can be implemented using the idea of dependency injection.
func Init(ds datastore.DataStore, addonCacheTime time.Duration) {
	clusterUsecase := usecase.NewClusterUsecase(ds)
	workflowUsecase := usecase.NewWorkflowUsecase(ds)
	projectUsecase := usecase.NewProjectUsecase(ds)
	deliveryTargetUsecase := usecase.NewDeliveryTargetUsecase(ds, projectUsecase)
	oamApplicationUsecase := usecase.NewOAMApplicationUsecase()
	velaQLUsecase := usecase.NewVelaQLUsecase()
	definitionUsecase := usecase.NewDefinitionUsecase()
	addonUsecase := usecase.NewAddonUsecase(addonCacheTime)
	envBindingUsecase := usecase.NewEnvBindingUsecase(ds, workflowUsecase, definitionUsecase)
	applicationUsecase := usecase.NewApplicationUsecase(ds, workflowUsecase, envBindingUsecase, deliveryTargetUsecase, definitionUsecase, projectUsecase)
	RegistWebService(NewClusterWebService(clusterUsecase))
	RegistWebService(NewApplicationWebService(applicationUsecase, envBindingUsecase, workflowUsecase))
	RegistWebService(NewProjectWebService(projectUsecase))
	RegistWebService(NewDefinitionWebservice(definitionUsecase))
	RegistWebService(NewAddonWebService(addonUsecase))
	RegistWebService(NewEnabledAddonWebService(addonUsecase))
	RegistWebService(NewAddonRegistryWebService(addonUsecase))
	RegistWebService(NewOAMApplication(oamApplicationUsecase))
	RegistWebService(&policyDefinitionWebservice{})
	RegistWebService(NewDeliveryTargetWebService(deliveryTargetUsecase, applicationUsecase))
	RegistWebService(NewVelaQLWebService(velaQLUsecase))
}
