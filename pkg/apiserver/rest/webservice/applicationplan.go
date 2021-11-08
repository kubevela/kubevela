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

type applicationPlanWebService struct {
	applicationUsecase usecase.ApplicationUsecase
}

// NewApplicationPlanWebService new application manage webservice
func NewApplicationPlanWebService(applicationUsecase usecase.ApplicationUsecase) WebService {
	return &applicationPlanWebService{
		applicationUsecase: applicationUsecase,
	}
}

func (c *applicationPlanWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/applicationplans").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for application manage")

	tags := []string{"applicationplan"}

	ws.Route(ws.GET("/").To(c.listApplicationPlans).
		Doc("list all application plans").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Param(ws.QueryParameter("namespace", "Namespace-based search").DataType("string")).
		Param(ws.QueryParameter("cluster", "Cluster-based search").DataType("string")).
		Returns(200, "", apis.ListApplicationPlanResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ListApplicationPlanResponse{}))

	ws.Route(ws.POST("/").To(c.createApplicationPlan).
		Doc("create one application plan").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateApplicationPlanRequest{}).
		Returns(200, "", apis.ApplicationPlanBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationPlanBase{}))

	ws.Route(ws.DELETE("/{name}").To(c.deleteApplicationPlan).
		Doc("delete one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{name}").To(c.detailApplicationPlan).
		Doc("detail one application plan").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Returns(200, "", apis.DetailApplicationPlanResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailApplicationPlanResponse{}))

	ws.Route(ws.PUT("/{name}").To(c.updateApplicationPlan).
		Doc("update one application plan").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Reads(apis.UpdateApplicationPlanRequest{}).
		Returns(200, "", apis.ApplicationPlanBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationPlanBase{}))

	ws.Route(ws.PUT("/{name}/envs/{envName}").To(c.updateApplicationEnvBinding).
		Doc("set application plan differences in the specified environment").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the application plan").DataType("string")).
		Reads(apis.PutApplicationPlanEnvRequest{}).
		Returns(200, "", apis.EnvBind{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EnvBind{}))

	ws.Route(ws.POST("/{name}/envs").To(c.createApplicationEnv).
		Doc("creating an application environment plan").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Reads(apis.CreateApplicationEnvPlanRequest{}).
		Returns(200, "", apis.EnvBind{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.DELETE("/{name}/envs/{envName}").To(c.deleteApplicationEnv).
		Doc("delete an application environment plan").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the application plan").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(404, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.POST("/{name}/template").To(c.publishApplicationTemplate).
		Doc("create one application template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Reads(apis.CreateApplicationTemplateRequest{}).
		Returns(200, "", apis.ApplicationTemplateBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationTemplateBase{}))

	ws.Route(ws.POST("/{name}/deploy").To(c.deployApplication).
		Doc("deploy or upgrade the application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Returns(200, "", apis.ApplicationDeployRequest{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationDeployResponse{}))

	ws.Route(ws.GET("/{name}/componentplans").To(c.listApplicationComponents).
		Doc("gets the componentplan topology of the application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Param(ws.QueryParameter("envName", "list components that deployed in define env").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ComponentPlanListResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ComponentPlanListResponse{}))

	ws.Route(ws.POST("/{name}/componentplans").To(c.createComponent).
		Doc("create component plan for application plan").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateComponentPlanRequest{}).
		Returns(200, "", apis.ComponentPlanBase{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ComponentPlanBase{}))

	ws.Route(ws.GET("/{name}/componentplans/{componentName}").To(c.detailComponent).
		Doc("detail component plan for application plan").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DetailComponentPlanResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailComponentPlanResponse{}))

	ws.Route(ws.GET("/{name}/policies").To(c.listApplicationPolicies).
		Doc("list policy for application").
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("name", "identifier of the application plan").DataType("string")).
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

func (c *applicationPlanWebService) appCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app, err := c.applicationUsecase.GetApplicationPlan(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplication, app))
	chain.ProcessFilter(req, res)
}

func (c *applicationPlanWebService) envCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	envBinding, err := c.applicationUsecase.GetApplicationPlanEnvBindingPolicy(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	for _, env := range envBinding.Envs {
		if env.Name == req.PathParameter("envName") {
			req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplicationEnvBinding, env))
			chain.ProcessFilter(req, res)
			return
		}
	}
	bcode.ReturnError(req, res, bcode.ErrApplicationNotEnv)
}

func (c *applicationPlanWebService) createApplicationPlan(req *restful.Request, res *restful.Response) {
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
	appBase, err := c.applicationUsecase.CreateApplicationPlan(req.Request.Context(), createReq)
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

func (c *applicationPlanWebService) listApplicationPlans(req *restful.Request, res *restful.Response) {
	apps, err := c.applicationUsecase.ListApplicationPlans(req.Request.Context(), apis.ListApplicatioPlanOptions{
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

func (c *applicationPlanWebService) detailApplicationPlan(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	detail, err := c.applicationUsecase.DetailApplicationPlan(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationPlanWebService) publishApplicationTemplate(req *restful.Request, res *restful.Response) {
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
func (c *applicationPlanWebService) deployApplication(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) deleteApplicationPlan(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	err := c.applicationUsecase.DeleteApplicationPlan(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationPlanWebService) listApplicationComponents(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	components, err := c.applicationUsecase.ListComponents(req.Request.Context(), app, apis.ListApplicationComponentOptions{
		EnvName: req.QueryParameter("envName"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ComponentPlanListResponse{ComponentPlans: components}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationPlanWebService) createComponent(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) detailComponent(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) createApplicationPolicy(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) listApplicationPolicies(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) detailApplicationPolicy(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) deleteApplicationPolicy(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) updateApplicationPolicy(req *restful.Request, res *restful.Response) {
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

func (c *applicationPlanWebService) updateApplicationEnvBinding(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	// Verify the validity of parameters
	var updateReq apis.PutApplicationPlanEnvRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	diff, err := c.applicationUsecase.UpdateApplicationEnvBindingPlan(req.Request.Context(), app, req.PathParameter("envName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(diff); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationPlanWebService) updateApplicationPlan(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	// Verify the validity of parameters
	var updateReq apis.UpdateApplicationPlanRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.applicationUsecase.UpdateApplicationPlan(req.Request.Context(), app, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationPlanWebService) createApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	// Verify the validity of parameters
	var createReq apis.CreateApplicationEnvPlanRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.applicationUsecase.CreateApplicationEnvBindingPlan(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationPlanWebService) deleteApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.ApplicationPlan)
	err := c.applicationUsecase.DeleteApplicationEnvBindingPlan(req.Request.Context(), app, req.PathParameter("envName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
