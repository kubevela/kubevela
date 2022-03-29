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
	"context"
	"log"
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

// RegisterWebService register webservice
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

// Init inits all webservice, pass in the required parameter object.
// It can be implemented using the idea of dependency injection.
func Init(ctx context.Context, ds datastore.DataStore, addonCacheTime time.Duration, initDatabase bool) map[string]interface{} {
	clusterUsecase := usecase.NewClusterUsecase(ds)
	rbacUsecase := usecase.NewRBACUsecase(ds)
	projectUsecase := usecase.NewProjectUsecase(ds, rbacUsecase)
	envUsecase := usecase.NewEnvUsecase(ds, projectUsecase)
	targetUsecase := usecase.NewTargetUsecase(ds)
	workflowUsecase := usecase.NewWorkflowUsecase(ds, envUsecase)
	oamApplicationUsecase := usecase.NewOAMApplicationUsecase()
	velaQLUsecase := usecase.NewVelaQLUsecase()
	definitionUsecase := usecase.NewDefinitionUsecase()
	addonUsecase := usecase.NewAddonUsecase(addonCacheTime)
	envBindingUsecase := usecase.NewEnvBindingUsecase(ds, workflowUsecase, definitionUsecase, envUsecase)
	applicationUsecase := usecase.NewApplicationUsecase(ds, workflowUsecase, envBindingUsecase, envUsecase, targetUsecase, definitionUsecase, projectUsecase)
	webhookUsecase := usecase.NewWebhookUsecase(ds, applicationUsecase)
	systemInfoUsecase := usecase.NewSystemInfoUsecase(ds)
	helmUsecase := usecase.NewHelmUsecase()
	userUsecase := usecase.NewUserUsecase(ds, projectUsecase, systemInfoUsecase, rbacUsecase)
	authenticationUsecase := usecase.NewAuthenticationUsecase(ds, systemInfoUsecase, userUsecase)
	// Modules that require default data initialization, Call it here in order
	if initDatabase {
		initData(ctx, userUsecase, rbacUsecase, projectUsecase, targetUsecase)
	}

	// Application
	RegisterWebService(NewApplicationWebService(applicationUsecase, envBindingUsecase, workflowUsecase, rbacUsecase))
	RegisterWebService(NewProjectWebService(projectUsecase, rbacUsecase, targetUsecase))
	RegisterWebService(NewEnvWebService(envUsecase, applicationUsecase, rbacUsecase))

	// Extension
	RegisterWebService(NewDefinitionWebservice(definitionUsecase, rbacUsecase))
	RegisterWebService(NewAddonWebService(addonUsecase, rbacUsecase, clusterUsecase))
	RegisterWebService(NewEnabledAddonWebService(addonUsecase, rbacUsecase))
	RegisterWebService(NewAddonRegistryWebService(addonUsecase, rbacUsecase))

	// Resources
	RegisterWebService(NewClusterWebService(clusterUsecase, rbacUsecase))
	RegisterWebService(NewOAMApplication(oamApplicationUsecase, rbacUsecase))
	RegisterWebService(&policyDefinitionWebservice{})
	RegisterWebService(&payloadTypesWebservice{})
	RegisterWebService(NewTargetWebService(targetUsecase, applicationUsecase, rbacUsecase))
	RegisterWebService(NewVelaQLWebService(velaQLUsecase, rbacUsecase))
	RegisterWebService(NewWebhookWebService(webhookUsecase, applicationUsecase))
	RegisterWebService(NewHelmWebService(helmUsecase))

	// Authentication
	RegisterWebService(NewAuthenticationWebService(authenticationUsecase, userUsecase))
	RegisterWebService(NewUserWebService(userUsecase, rbacUsecase))
	RegisterWebService(NewSystemInfoWebService(systemInfoUsecase, rbacUsecase))

	// RBAC
	RegisterWebService(NewRBACWebService(rbacUsecase))

	// return some usecase instance
	return map[string]interface{}{"workflow": workflowUsecase, "project": projectUsecase}
}

// InitUsecase the usecase set that needs init data
type InitUsecase interface {
	Init(ctx context.Context) error
}

func initData(ctx context.Context, inits ...InitUsecase) {
	for _, init := range inits {
		if err := init.Init(ctx); err != nil {
			log.Fatalf("database init failure %s", err.Error())
		}
	}
}
