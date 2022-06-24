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

package api

import (
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

// NewAddonRegistryAPIInterface returns addon registry web service
func NewAddonRegistryAPIInterface() Interface {
	return &addonRegistryAPIInterface{}
}

type addonRegistryAPIInterface struct {
	AddonService service.AddonService `inject:""`
	RbacService  service.RBACService  `inject:""`
}

func (s *addonRegistryAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/addon_registries").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for addon registry management")

	tags := []string{"addon_registry"}

	// Create
	ws.Route(ws.POST("/").To(s.createAddonRegistry).
		Doc("create an addon registry").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateAddonRegistryRequest{}).
		Filter(s.RbacService.CheckPerm("addonRegistry", "create")).
		Returns(200, "OK", apis.AddonRegistry{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.AddonRegistry{}))

	ws.Route(ws.GET("/").To(s.listAddonRegistry).
		Doc("list all addon registry").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(s.RbacService.CheckPerm("addonRegistry", "list")).
		Returns(200, "OK", apis.ListAddonRegistryResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListAddonRegistryResponse{}))

	// Delete
	ws.Route(ws.DELETE("/{addonRegName}").To(s.deleteAddonRegistry).
		Doc("delete an addon registry").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("addonRegName", "identifier of the addon registry").DataType("string")).
		Returns(200, "OK", apis.AddonRegistry{}).
		Filter(s.RbacService.CheckPerm("addonRegistry", "delete")).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.AddonRegistry{}))

	ws.Route(ws.PUT("/{addonRegName}").To(s.updateAddonRegistry).
		Doc("update an addon registry").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateAddonRegistryRequest{}).
		Filter(s.RbacService.CheckPerm("addonRegistry", "update")).
		Param(ws.PathParameter("addonRegName", "identifier of the addon registry").DataType("string")).
		Returns(200, "OK", apis.AddonRegistry{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.AddonRegistry{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (s *addonRegistryAPIInterface) createAddonRegistry(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateAddonRegistryRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Call the domain layer code
	meta, err := s.AddonService.CreateAddonRegistry(req.Request.Context(), createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(meta); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonRegistryAPIInterface) deleteAddonRegistry(req *restful.Request, res *restful.Response) {
	r, err := s.AddonService.GetAddonRegistry(req.Request.Context(), req.PathParameter("addonRegName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = s.AddonService.DeleteAddonRegistry(req.Request.Context(), r.Name)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(r); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonRegistryAPIInterface) listAddonRegistry(req *restful.Request, res *restful.Response) {
	registries, err := s.AddonService.ListAddonRegistries(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListAddonRegistryResponse{Registries: registries}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonRegistryAPIInterface) updateAddonRegistry(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateAddonRegistryRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	meta, err := s.AddonService.UpdateAddonRegistry(req.Request.Context(), req.PathParameter("addonRegName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(meta); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
