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
}

// NewApplicationWebService new application manage webservice
func NewApplicationWebService(applicationUsecase usecase.ApplicationUsecase) WebService {
	return &applicationWebService{
		applicationUsecase: applicationUsecase,
	}
}

func (c *applicationWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/applicationplans").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for application manage")

	tags := []string{"application"}

	ws.Route(ws.GET("/").To(c.listApplications).
		Doc("list all applications").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Param(ws.QueryParameter("namespace", "Namespace-based search").DataType("string")).
		Param(ws.QueryParameter("cluster", "Cluster-based search").DataType("string")).
		Returns(200, "", apis.ListApplicationPlanResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ListApplicationPlanResponse{}))

	ws.Route(ws.POST("/").To(c.createApplication).
		Doc("create one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateApplicationPlanRequest{}).
		Returns(200, "", apis.ApplicationPlanBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationPlanBase{}))

	ws.Route(ws.DELETE("/{name}").To(c.deleteApplication).
		Doc("delete one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{name}").To(c.detailApplication).
		Doc("detail one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Returns(200, "", apis.DetailApplicationPlanResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailApplicationPlanResponse{}))

	ws.Route(ws.POST("/{name}/template").To(c.publishApplicationTemplate).
		Doc("create one application template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Reads(apis.CreateApplicationTemplateRequest{}).
		Returns(200, "", apis.ApplicationTemplateBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationTemplateBase{}))

	ws.Route(ws.POST("/{name}/deploy").To(c.deployApplication).
		Doc("deploy or upgrade the application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Returns(200, "", apis.ApplicationDeployRequest{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationDeployResponse{}))

	ws.Route(ws.GET("/{name}/componentplans").To(c.listApplicationComponents).
		Doc("gets the componentplan topology of the application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.QueryParameter("envName", "list components that deployed in define env").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ComponentPlanListResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ComponentPlanListResponse{}))

	ws.Route(ws.POST("/{name}/componentplans").To(c.createComponent).
		Doc("create component plan for application plan").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateComponentPlanRequest{}).
		Returns(200, "", apis.ComponentPlanBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ComponentPlanBase{}))

	ws.Route(ws.GET("/{name}/componentplans/{componentName}").To(c.detailComponent).
		Doc("detail component plan for application plan").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DetailComponentPlanResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailComponentPlanResponse{}))

	ws.Route(ws.GET("/{name}/policies").To(c.listApplicationPolicies).
		Doc("list policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
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
	return ws
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

func (c *applicationWebService) createApplication(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateApplicationPlanRequest
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
	apps, err := c.applicationUsecase.ListApplications(req.Request.Context(), apis.ListApplicatioPlanOptions{
		Namespace: req.QueryParameter("namespace"),
		Cluster:   req.QueryParameter("cluster"),
		Query:     req.QueryParameter("query"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationPlanResponse{ApplicationPlans: apps}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) detailApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	components, err := c.applicationUsecase.ListComponents(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ComponentPlanListResponse{ComponentPlans: components}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationWebService) createComponent(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	// Verify the validity of parameters
	var createReq apis.CreateComponentPlanRequest
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
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
