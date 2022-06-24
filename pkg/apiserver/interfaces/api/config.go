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
	ws.Path(versionPrefix+"/config_types").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for configuration management")

	tags := []string{"config"}

	ws.Route(ws.GET("/").To(s.listConfigTypes).
		Doc("list all config types").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("configType", "list")).
		Param(ws.QueryParameter("query", "Fuzzy search based on name and description.").DataType("string")).
		Returns(200, "OK", []apis.ConfigType{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]apis.ConfigType{}))

	ws.Route(ws.GET("/{configType}").To(s.getConfigType).
		Doc("get a config type").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("configType", "get")).
		Param(ws.PathParameter("configType", "identifier of the config type").DataType("string")).
		Returns(200, "OK", apis.ConfigType{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ConfigType{}))

	ws.Route(ws.POST("/{configType}").To(s.createConfig).
		Doc("create or update a config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("configType", "create")).
		Param(ws.PathParameter("configType", "identifier of the config type").DataType("string")).
		Reads(apis.CreateConfigRequest{}).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{configType}/configs").To(s.getConfigs).
		Doc("get configs from a config type").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "list")).
		Param(ws.PathParameter("configType", "identifier of the config").DataType("string")).
		Returns(200, "OK", []*apis.Config{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ConfigType{}))

	ws.Route(ws.GET("/{configType}/configs/{name}").To(s.getConfig).
		Doc("get a config from a config type").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "get")).
		Param(ws.PathParameter("configType", "identifier of the config type").DataType("string")).
		Param(ws.PathParameter("name", "identifier of the config").DataType("string")).
		Returns(200, "OK", []*apis.Config{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ConfigType{}))

	ws.Route(ws.DELETE("/{configType}/configs/{name}").To(s.deleteConfig).
		Doc("delete a config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("config", "delete")).
		Param(ws.PathParameter("configType", "identifier of the config type").DataType("string")).
		Param(ws.PathParameter("name", "identifier of the config").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (s *configAPIInterface) listConfigTypes(req *restful.Request, res *restful.Response) {
	types, err := s.ConfigService.ListConfigTypes(req.Request.Context(), req.QueryParameter("query"))
	if len(types) == 0 && err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(types)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *configAPIInterface) getConfigType(req *restful.Request, res *restful.Response) {
	t, err := s.ConfigService.GetConfigType(req.Request.Context(), req.PathParameter("configType"))
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

	err := s.ConfigService.CreateConfig(req.Request.Context(), createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *configAPIInterface) getConfigs(req *restful.Request, res *restful.Response) {
	configs, err := s.ConfigService.GetConfigs(req.Request.Context(), req.PathParameter("configType"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(configs)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *configAPIInterface) getConfig(req *restful.Request, res *restful.Response) {
	t, err := s.ConfigService.GetConfig(req.Request.Context(), req.PathParameter("configType"), req.PathParameter("name"))
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
	err := s.ConfigService.DeleteConfig(req.Request.Context(), req.PathParameter("configType"), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
