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
	"github.com/oam-dev/kubevela/pkg/config"
)

// ConfigAPIInterface returns config web service
func ConfigAPIInterface() Interface {
	return &configAPIInterface{}
}

type configAPIInterface struct {
	ConfigService service.ConfigService `inject:""`
	RbacService   service.RBACService   `inject:""`
}

func (s *configAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/configs").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for config management")

	tags := []string{"config"}

	ws.Route(ws.POST("/").To(s.createConfig).
		Doc("create or update a config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "create")).
		Reads(apis.CreateConfigRequest{}).
		Returns(200, "OK", apis.Config{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.Config{}))

	ws.Route(ws.GET("/").To(s.getConfigs).
		Doc("list all configs that belong to the system scope").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "list")).
		Param(ws.QueryParameter("template", "the name of the template").DataType("string")).
		Returns(200, "OK", apis.ListConfigResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListConfigResponse{}))

	ws.Route(ws.GET("/{configName}").To(s.getConfig).
		Doc("detail a config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "get")).
		Param(ws.PathParameter("configName", "identifier of the config").DataType("string")).
		Returns(200, "OK", []*apis.Config{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.Config{}))

	ws.Route(ws.PUT("/{configName}").To(s.updateConfig).
		Doc("update a config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "update")).
		Param(ws.PathParameter("configName", "identifier of the config").DataType("string")).
		Returns(200, "OK", []*apis.UpdateConfigRequest{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.UpdateConfigRequest{}))

	ws.Route(ws.DELETE("/{configName}").To(s.deleteConfig).
		Doc("delete a config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "delete")).
		Param(ws.PathParameter("configName", "identifier of the config").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

// ConfigTemplateAPIInterface returns config web service
func ConfigTemplateAPIInterface() Interface {
	return &configTemplateAPIInterface{}
}

type configTemplateAPIInterface struct {
	ConfigService service.ConfigService `inject:""`
	RbacService   service.RBACService   `inject:""`
}

func (s *configTemplateAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/config_templates").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for config management")

	tags := []string{"config"}

	ws.Route(ws.GET("/").To(s.listConfigTemplates).
		Doc("List all config templates from the system namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "list")).
		Returns(200, "OK", apis.ListConfigTemplateResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]apis.ListConfigTemplateResponse{}))

	ws.Route(ws.GET("{templateName}").To(s.getConfigTemplate).
		Doc("Detail a template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "get")).
		Param(ws.PathParameter("templateName", "identifier of the config template").DataType("string")).
		Param(ws.QueryParameter("namespace", "the name of the namespace").DataType("string")).
		Returns(200, "OK", apis.ConfigTemplateDetail{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ConfigTemplateDetail{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (s *configTemplateAPIInterface) listConfigTemplates(req *restful.Request, res *restful.Response) {
	templates, err := s.ConfigService.ListTemplates(req.Request.Context(), "", "")
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListConfigTemplateResponse{Templates: templates})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *configTemplateAPIInterface) getConfigTemplate(req *restful.Request, res *restful.Response) {
	t, err := s.ConfigService.GetTemplate(req.Request.Context(), config.NamespacedName{
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

func (s *configAPIInterface) createConfig(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateConfigRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	config, err := s.ConfigService.CreateConfig(req.Request.Context(), "", createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(config); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *configAPIInterface) updateConfig(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateConfigRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	config, err := s.ConfigService.UpdateConfig(req.Request.Context(), "", req.PathParameter("configName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(config); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *configAPIInterface) getConfigs(req *restful.Request, res *restful.Response) {
	configs, err := s.ConfigService.ListConfigs(req.Request.Context(), "", req.QueryParameter("template"), true)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(apis.ListConfigResponse{Configs: configs})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *configAPIInterface) getConfig(req *restful.Request, res *restful.Response) {
	t, err := s.ConfigService.GetConfig(req.Request.Context(), "", req.PathParameter("configName"))
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

func (s *configAPIInterface) deleteConfig(req *restful.Request, res *restful.Response) {
	err := s.ConfigService.DeleteConfig(req.Request.Context(), "", req.PathParameter("configName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
