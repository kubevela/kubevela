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

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type envBindingWebService struct {
	envBindingUsecase  usecase.EnvBindingUsecase
	applicationUsecase usecase.ApplicationUsecase
}

// NewEnvBindingWebService new envBinding manage webservice
func NewEnvBindingWebService(applicationUsecase usecase.ApplicationUsecase, envBindingUsecase usecase.EnvBindingUsecase) WebService {
	return &envBindingWebService{
		envBindingUsecase:  envBindingUsecase,
		applicationUsecase: applicationUsecase,
	}
}

func (c *envBindingWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/envbingings").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for application manage")

	tags := []string{"application-envbingings"}

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

func (c *envBindingWebService) updateApplicationEnv(req *restful.Request, res *restful.Response) {
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

func (c *envBindingWebService) createApplicationEnv(req *restful.Request, res *restful.Response) {
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

func (c *envBindingWebService) deleteApplicationEnv(req *restful.Request, res *restful.Response) {
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

func (c *envBindingWebService) appCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app, err := c.applicationUsecase.GetApplication(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplication, app))
	chain.ProcessFilter(req, res)
}

func (c *envBindingWebService) envCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
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
