/*
Copyright 2022 The KubeVela Authors.

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
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/integration"
)

// IntegrationAPIInterface returns integration web service
func IntegrationAPIInterface() Interface {
	return &integrationAPIInterface{}
}

type integrationAPIInterface struct {
	IntegrationService service.IntegrationService `inject:""`
	RbacService        service.RBACService        `inject:""`
}

func (s *integrationAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/integrations").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for integration management")

	tags := []string{"integration"}

	ws.Route(ws.POST("/").To(s.createIntegration).
		Doc("create or update a integration").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("integration", "create")).
		Reads(apis.CreateIntegrationRequest{}).
		Returns(200, "OK", apis.Integration{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.Integration{}))

	ws.Route(ws.GET("/").To(s.getIntegrations).
		Doc("list all integrations that belong to the system scope").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("integration", "list")).
		Param(ws.QueryParameter("template", "the name of the template").DataType("string")).
		Returns(200, "OK", apis.ListIntegrationResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListIntegrationResponse{}))

	ws.Route(ws.GET("/{integrationName}").To(s.getIntegration).
		Doc("detail a integration").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("integration", "get")).
		Param(ws.PathParameter("integrationName", "identifier of the integration").DataType("string")).
		Returns(200, "OK", []*apis.Integration{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.Integration{}))

	ws.Route(ws.PUT("/{integrationName}").To(s.updateIntegration).
		Doc("update a integration").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("integration", "update")).
		Param(ws.PathParameter("integrationName", "identifier of the integration").DataType("string")).
		Returns(200, "OK", []*apis.UpdateIntegrationRequest{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.UpdateIntegrationRequest{}))

	ws.Route(ws.DELETE("/{integrationName}").To(s.deleteIntegration).
		Doc("delete a integration").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("integration", "delete")).
		Param(ws.PathParameter("integrationName", "identifier of the integration").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

// IntegrationTemplateAPIInterface returns integration web service
func IntegrationTemplateAPIInterface() Interface {
	return &integrationTemplateAPIInterface{}
}

type integrationTemplateAPIInterface struct {
	IntegrationService service.IntegrationService `inject:""`
	RbacService        service.RBACService        `inject:""`
}

func (s *integrationTemplateAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/integration_templates").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for integration management")

	tags := []string{"integration"}

	ws.Route(ws.GET("/").To(s.listIntegrationTemplates).
		Doc("List all integration templates from the system namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("integrationTemplate", "list")).
		Returns(200, "OK", apis.ListIntegrationTemplateResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]apis.ListIntegrationTemplateResponse{}))

	ws.Route(ws.GET("{templateName}").To(s.getIntegrationTemplate).
		Doc("Detail a template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("integrationTemplate", "get")).
		Param(ws.PathParameter("templateName", "identifier of the integration template").DataType("string")).
		Param(ws.QueryParameter("namespace", "the name of the namespace").DataType("string")).
		Returns(200, "OK", apis.IntegrationTemplateDetail{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.IntegrationTemplateDetail{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (s *integrationTemplateAPIInterface) listIntegrationTemplates(req *restful.Request, res *restful.Response) {
	templates, err := s.IntegrationService.ListTemplates(req.Request.Context(), "", "")
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListIntegrationTemplateResponse{Templates: templates})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *integrationTemplateAPIInterface) getIntegrationTemplate(req *restful.Request, res *restful.Response) {

	t, err := s.IntegrationService.GetTemplate(req.Request.Context(), integration.TemplateBase{
		Name:      req.PathParameter("templateName"),
		Namespace: req.QueryParameter("namespace"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(t)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *integrationAPIInterface) createIntegration(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateIntegrationRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	integration, err := s.IntegrationService.CreateIntegration(req.Request.Context(), "", createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(integration); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *integrationAPIInterface) updateIntegration(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateIntegrationRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	integration, err := s.IntegrationService.UpdateIntegration(req.Request.Context(), "", req.PathParameter("integrationName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(integration); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *integrationAPIInterface) getIntegrations(req *restful.Request, res *restful.Response) {
	integrations, err := s.IntegrationService.ListIntegrations(req.Request.Context(), "", req.QueryParameter("template"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListIntegrationResponse{Integrations: integrations})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *integrationAPIInterface) getIntegration(req *restful.Request, res *restful.Response) {
	t, err := s.IntegrationService.GetIntegration(req.Request.Context(), "", req.PathParameter("integrationName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(t)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *integrationAPIInterface) deleteIntegration(req *restful.Request, res *restful.Response) {
	err := s.IntegrationService.DeleteIntegration(req.Request.Context(), "", req.PathParameter("integrationName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
