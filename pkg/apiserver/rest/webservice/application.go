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

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	restful "github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type applicationWebService struct {
	applicationUsecase usecase.ApplicationUsecase
	envBindingUsecase  usecase.EnvBindingUsecase
}

// NewApplicationWebService new application manage webservice
func NewApplicationWebService(applicationUsecase usecase.ApplicationUsecase, envBindingUsecase usecase.EnvBindingUsecase) WebService {
	return &applicationWebService{
		applicationUsecase: applicationUsecase,
		envBindingUsecase:  envBindingUsecase,
	}
}

func (c *applicationWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/applications").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for application manage")

	tags := []string{"application"}

	ws.Route(ws.GET("/").To(c.listApplications).
		Doc("list all applications").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Param(ws.QueryParameter("namespace", "The namespace of the managed cluster").DataType("string")).
		Param(ws.QueryParameter("target", "Name of the application delivery target").DataType("string")).
		Returns(200, "", apis.ListApplicationResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ListApplicationResponse{}))

	ws.Route(ws.POST("/").To(c.createApplication).
		Doc("create one application ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateApplicationRequest{}).
		Returns(200, "", apis.ApplicationBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationBase{}))

	ws.Route(ws.DELETE("/{name}").To(c.deleteApplication).
		Doc("delete one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{name}").To(c.detailApplication).
		Doc("detail one application ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Returns(200, "", apis.DetailApplicationResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailApplicationResponse{}))

	ws.Route(ws.GET("/{name}/envs/{envName}/status").To(c.getApplicationStatus).
		Doc("get application status").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the application envbinding").DataType("string")).
		Returns(200, "", apis.ApplicationStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationStatusResponse{}))

	ws.Route(ws.PUT("/{name}").To(c.updateApplication).
		Doc("update one application ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Reads(apis.UpdateApplicationRequest{}).
		Returns(200, "", apis.ApplicationBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationBase{}))

	ws.Route(ws.POST("/{name}/template").To(c.publishApplicationTemplate).
		Doc("create one application template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Reads(apis.CreateApplicationTemplateRequest{}).
		Returns(200, "", apis.ApplicationTemplateBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationTemplateBase{}))

	ws.Route(ws.POST("/{name}/deploy").To(c.deployApplication).
		Doc("deploy or upgrade the application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Returns(200, "", apis.ApplicationDeployRequest{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationDeployResponse{}))

	ws.Route(ws.GET("/{name}/components").To(c.listApplicationComponents).
		Doc("gets the list of application components").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Param(ws.QueryParameter("envName", "list components that deployed in define env").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ComponentListResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ComponentListResponse{}))

	ws.Route(ws.POST("/{name}/components").To(c.createComponent).
		Doc("create component  for application ").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateComponentRequest{}).
		Returns(200, "", apis.ComponentBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ComponentBase{}))

	ws.Route(ws.GET("/{name}/components/{componentName}").To(c.detailComponent).
		Doc("detail component  for application ").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DetailComponentResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailComponentResponse{}))

	ws.Route(ws.GET("/{name}/policies").To(c.listApplicationPolicies).
		Doc("list policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ListApplicationPolicy{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ListApplicationPolicy{}))

	ws.Route(ws.POST("/{name}/policies").To(c.createApplicationPolicy).
		Doc("create policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreatePolicyRequest{}).
		Returns(200, "", apis.PolicyBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.PolicyBase{}))

	ws.Route(ws.GET("/{name}/policies/{policyName}").To(c.detailApplicationPolicy).
		Doc("detail policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DetailPolicyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailPolicyResponse{}))

	ws.Route(ws.DELETE("/{name}/policies/{policyName}").To(c.deleteApplicationPolicy).
		Doc("detail policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.PUT("/{name}/policies/{policyName}").To(c.updateApplicationPolicy).
		Doc("update policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdatePolicyRequest{}).
		Returns(200, "", apis.DetailPolicyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailPolicyResponse{}))

	ws.Route(ws.POST("/{name}/components/{compName}/traits").To(c.addApplicationTrait).
		Doc("add trait for a component").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateApplicationTraitRequest{}).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationTrait{}))

	ws.Route(ws.PUT("/{name}/components/{compName}/traits/{traitType}").To(c.updateApplicationTrait).
		Doc("update trait from a component").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Param(ws.PathParameter("traitType", "identifier of the type of trait").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateApplicationTraitRequest{}).
		Returns(200, "", apis.ApplicationTrait{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationTrait{}))

	ws.Route(ws.DELETE("/{name}/components/{compName}/traits/{traitType}").To(c.deleteApplicationTrait).
		Doc("delete trait from a component").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Param(ws.PathParameter("traitType", "identifier of the type of trait").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ApplicationTrait{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{name}/revisions").To(c.listApplicationRevisions).
		Doc("list revisions for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Param(ws.QueryParameter("envName", "query identifier of the env").DataType("string")).
		Param(ws.QueryParameter("status", "query identifier of the status").DataType("string")).
		Param(ws.QueryParameter("page", "query the page number").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "query the page size number").DataType("integer")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ListRevisionsResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ListRevisionsResponse{}))

	ws.Route(ws.GET("/{name}/revisions/{revision}").To(c.detailApplicationRevision).
		Doc("detail revision for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("revision", "identifier of the application revision").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DetailRevisionResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailRevisionResponse{}))

	ws.Route(ws.GET("/{name}/envs").To(c.listApplicationEnvs).
		Doc("list policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ListApplicationEnvBinding{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ListApplicationEnvBinding{}))

	ws.Route(ws.POST("/{name}/envs").To(c.createApplicationEnv).
		Doc("creating an application environment ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Reads(apis.CreateApplicationEnvRequest{}).
		Returns(200, "", apis.EnvBinding{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.PUT("/{name}/envs/{envName}").To(c.updateApplicationEnv).
		Doc("set application  differences in the specified environment").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the envBinding ").DataType("string")).
		Reads(apis.PutApplicationEnvRequest{}).
		Returns(200, "", apis.EnvBinding{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EnvBinding{}))

	ws.Route(ws.DELETE("/{name}/envs/{envName}").To(c.deleteApplicationEnv).
		Doc("delete an application environment ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the envBinding ").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(404, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	return ws
}

func (c *applicationWebService) createApplication(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateApplicationRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the usecase layer code
	appBase, err := c.applicationUsecase.CreateApplication(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create application failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(appBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) listApplications(req *restful.Request, res *restful.Response) {
	apps, err := c.applicationUsecase.ListApplications(req.Request.Context(), apis.ListApplicatioOptions{
		Namespace:  req.QueryParameter("namespace"),
		TargetName: req.QueryParameter("target"),
		Query:      req.QueryParameter("query"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationResponse{Applications: apps}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) detailApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.applicationUsecase.DetailApplication(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) publishApplicationTemplate(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	base, err := c.applicationUsecase.PublishApplicationTemplate(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

// deployApplication TODO: return event model
func (c *applicationWebService) deployApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.ApplicationDeployRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	deployRes, err := c.applicationUsecase.Deploy(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(deployRes); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) deleteApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.applicationUsecase.DeleteApplication(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) listApplicationComponents(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	components, err := c.applicationUsecase.ListComponents(req.Request.Context(), app, apis.ListApplicationComponentOptions{
		EnvName: req.QueryParameter("envName"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ComponentListResponse{Components: components}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) createComponent(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.CreateComponentRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.applicationUsecase.AddComponent(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) detailComponent(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.applicationUsecase.DetailComponent(req.Request.Context(), app, req.PathParameter("componentName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) createApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.CreatePolicyRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.applicationUsecase.AddPolicy(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) listApplicationPolicies(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	policies, err := c.applicationUsecase.ListPolicies(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationPolicy{Policies: policies}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) detailApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.applicationUsecase.DetailPolicy(req.Request.Context(), app, req.PathParameter("policyName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) deleteApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.applicationUsecase.DeletePolicy(req.Request.Context(), app, req.PathParameter("policyName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) updateApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var updateReq apis.UpdatePolicyRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	response, err := c.applicationUsecase.UpdatePolicy(req.Request.Context(), app, req.PathParameter("policyName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(response); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) updateApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var updateReq apis.UpdateApplicationRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.applicationUsecase.UpdateApplication(req.Request.Context(), app, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) addApplicationTrait(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	var createReq apis.CreateApplicationTraitRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	trait, err := c.applicationUsecase.CreateApplicationTrait(req.Request.Context(), app,
		&model.ApplicationComponent{Name: req.PathParameter("compName")}, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(trait); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) updateApplicationTrait(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	var updateReq apis.UpdateApplicationTraitRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	trait, err := c.applicationUsecase.UpdateApplicationTrait(req.Request.Context(), app,
		&model.ApplicationComponent{Name: req.PathParameter("compName")}, req.PathParameter("traitType"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(trait); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) deleteApplicationTrait(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.applicationUsecase.DeleteApplicationTrait(req.Request.Context(), app,
		&model.ApplicationComponent{Name: req.PathParameter("compName")}, req.PathParameter("traitType"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) getApplicationStatus(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	status, err := c.applicationUsecase.GetApplicationStatus(req.Request.Context(), app, req.PathParameter("envName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(apis.ApplicationStatusResponse{Status: status}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) listApplicationRevisions(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	revisions, err := c.applicationUsecase.ListRevisions(req.Request.Context(), req.PathParameter("name"), req.QueryParameter("envName"), req.QueryParameter("status"), page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(revisions); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) detailApplicationRevision(req *restful.Request, res *restful.Response) {
	detail, err := c.applicationUsecase.DetailRevision(req.Request.Context(), req.PathParameter("name"), req.PathParameter("revision"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) updateApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var updateReq apis.PutApplicationEnvRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	diff, err := c.envBindingUsecase.UpdateEnvBinding(req.Request.Context(), app, req.PathParameter("envName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(diff); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) listApplicationEnvs(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	envBindings, err := c.envBindingUsecase.GetEnvBindings(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationEnvBinding{EnvBindings: envBindings}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) createApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.CreateApplicationEnvRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.envBindingUsecase.CreateEnvBinding(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) deleteApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.envBindingUsecase.DeleteEnvBinding(req.Request.Context(), app, req.PathParameter("envName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) appCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app, err := c.applicationUsecase.GetApplication(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplication, app))
	chain.ProcessFilter(req, res)
}

func (c *applicationWebService) envCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	envBindings, err := c.envBindingUsecase.GetEnvBindings(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	for _, env := range envBindings {
		if env.Name == req.PathParameter("envName") {
			req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplicationEnvBinding, env))
			chain.ProcessFilter(req, res)
			return
		}
	}
	bcode.ReturnError(req, res, bcode.ErrApplicationNotEnv)
}

func (c *applicationWebService) resumeApplicationRevision(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	revision, err := c.applicationUsecase.ResumeRevision(req.Request.Context(), app, req.PathParameter("revision"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(revision); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) terminateApplicationRevision(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	revision, err := c.applicationUsecase.TerminateRevision(req.Request.Context(), app, req.PathParameter("revision"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(revision); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) rollbackApplicationRevision(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	revision, err := c.applicationUsecase.RollbackRevision(req.Request.Context(), app, req.PathParameter("revision"), req.PathParameter("rollback"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(revision); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
