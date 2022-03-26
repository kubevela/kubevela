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
	"strconv"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/apis/types"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// NewAddonWebService returns addon web service
func NewAddonWebService(u usecase.AddonHandler, rbacUsecase usecase.RBACUsecase) WebService {
	return &addonWebService{
		handler:     u,
		rbacUsecase: rbacUsecase,
	}
}

// NewEnabledAddonWebService returns enabled addon web service
func NewEnabledAddonWebService(u usecase.AddonHandler, rbacUsecase usecase.RBACUsecase) WebService {
	return &enabledAddonWebService{
		addonUsecase: u,
		rbacUsecase:  rbacUsecase,
	}
}

type addonWebService struct {
	rbacUsecase usecase.RBACUsecase
	handler     usecase.AddonHandler
}

func (s *addonWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/addons").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for addon management")

	tags := []string{"addon"}

	// List
	ws.Route(ws.GET("/").To(s.listAddons).
		Doc("list all addons").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.rbacUsecase.CheckPerm("addon", "list")).
		Param(ws.QueryParameter("registry", "filter addons from given registry").DataType("string")).
		Param(ws.QueryParameter("query", "Fuzzy search based on name and description.").DataType("string")).
		Returns(200, "OK", apis.ListAddonResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListAddonResponse{}))

	// GET
	ws.Route(ws.GET("/{addonName}").To(s.detailAddon).
		Doc("show details of an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.rbacUsecase.CheckPerm("addon", "detail")).
		Returns(200, "OK", apis.DetailAddonResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Param(ws.PathParameter("addonName", "addon name to query detail").DataType("string").Required(true)).
		Param(ws.QueryParameter("registry", "filter addons from given registry").DataType("string")).
		Writes(apis.DetailAddonResponse{}))

	// GET status
	ws.Route(ws.GET("/{addonName}/status").To(s.statusAddon).
		Doc("show status of an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.rbacUsecase.CheckPerm("addon", "detail")).
		Returns(200, "OK", apis.AddonStatusResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Param(ws.PathParameter("addonName", "addon name to query status").DataType("string").Required(true)).
		Writes(apis.AddonStatusResponse{}))

	// enable addon
	ws.Route(ws.POST("/{addonName}/enable").To(s.enableAddon).
		Doc("enable an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.EnableAddonRequest{}).
		Filter(s.rbacUsecase.CheckPerm("addon", "enable")).
		Returns(200, "OK", apis.AddonStatusResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Param(ws.PathParameter("addonName", "addon name to enable").DataType("string").Required(true)).
		Writes(apis.AddonStatusResponse{}))

	// disable addon
	ws.Route(ws.POST("/{addonName}/disable").To(s.disableAddon).
		Doc("disable an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.AddonStatusResponse{}).
		Filter(s.rbacUsecase.CheckPerm("addon", "disable")).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Param(ws.PathParameter("addonName", "addon name to enable").DataType("string").Required(true)).
		Param(ws.QueryParameter("force", "force disable an addon").DataType("boolean").Required(false)).
		Writes(apis.AddonStatusResponse{}))

	// update addon
	ws.Route(ws.PUT("/{addonName}/update").To(s.updateAddon).
		Doc("update an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.EnableAddonRequest{}).
		Returns(200, "OK", apis.AddonStatusResponse{}).
		Filter(s.rbacUsecase.CheckPerm("addon", "update")).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Param(ws.PathParameter("addonName", "addon name to update").DataType("string").Required(true)).
		Writes(apis.AddonStatusResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (s *addonWebService) listAddons(req *restful.Request, res *restful.Response) {
	detailAddons, err := s.handler.ListAddons(req.Request.Context(), req.QueryParameter("registry"), req.QueryParameter("query"))
	if len(detailAddons) == 0 && err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	var addons []*apis.AddonInfo

	for _, d := range detailAddons {
		addons = append(addons, &apis.AddonInfo{Meta: &d.Meta, RegistryName: d.RegistryName})
	}

	var message string
	if err != nil {
		message = err.Error()
	}
	err = res.WriteEntity(apis.ListAddonResponse{Addons: addons, Message: message})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonWebService) detailAddon(req *restful.Request, res *restful.Response) {
	name := req.PathParameter("addonName")
	addon, err := s.handler.GetAddon(req.Request.Context(), name, req.QueryParameter("registry"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	err = res.WriteEntity(addon)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

}

func (s *addonWebService) enableAddon(req *restful.Request, res *restful.Response) {
	var createReq apis.EnableAddonRequest
	var args []byte
	_, err := req.Request.Body.Read(args)
	if err == nil {
		err := req.ReadEntity(&createReq)
		if err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
		if err = validate.Struct(&createReq); err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
	}
	if createReq.Clusters != nil {
		createReq.Args[types.ClustersArg] = createReq.Clusters
	}

	name := req.PathParameter("addonName")
	err = s.handler.EnableAddon(req.Request.Context(), name, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	s.statusAddon(req, res)
}

func (s *addonWebService) disableAddon(req *restful.Request, res *restful.Response) {
	name := req.PathParameter("addonName")
	forceParam := req.QueryParameter("force")
	force, _ := strconv.ParseBool(forceParam)
	err := s.handler.DisableAddon(req.Request.Context(), name, force)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	s.statusAddon(req, res)
}

func (s *addonWebService) statusAddon(req *restful.Request, res *restful.Response) {
	name := req.PathParameter("addonName")
	status, err := s.handler.StatusAddon(req.Request.Context(), name)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	err = res.WriteEntity(*status)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonWebService) updateAddon(req *restful.Request, res *restful.Response) {
	var createReq apis.EnableAddonRequest
	var args []byte
	_, err := req.Request.Body.Read(args)
	if err == nil {
		err := req.ReadEntity(&createReq)
		if err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
		if err = validate.Struct(&createReq); err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
	}
	if createReq.Clusters != nil {
		createReq.Args[types.ClustersArg] = createReq.Clusters
	}

	name := req.PathParameter("addonName")
	err = s.handler.UpdateAddon(req.Request.Context(), name, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	s.statusAddon(req, res)
}

type enabledAddonWebService struct {
	addonUsecase usecase.AddonHandler
	rbacUsecase  usecase.RBACUsecase
}

func (s *enabledAddonWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/enabled_addon").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for addon management")

	tags := []string{"addon"}

	// List enabled addon from cluster
	ws.Route(ws.GET("/").To(s.list).
		Doc("list all addons").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.rbacUsecase.CheckPerm("addon", "list")).
		Param(ws.QueryParameter("registry", "filter addons from given registry").DataType("string")).
		Param(ws.QueryParameter("query", "Fuzzy search based on name and description.").DataType("string")).
		Returns(200, "OK", apis.ListAddonResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListAddonResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (s *enabledAddonWebService) list(req *restful.Request, res *restful.Response) {
	enabledAddons, err := s.addonUsecase.ListEnabledAddon(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	err = res.WriteEntity(apis.ListEnabledAddonResponse{EnabledAddons: enabledAddons})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
