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
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// NewAddonRegistryWebService returns addon registry web service
func NewAddonRegistryWebService(u usecase.AddonUsecase) WebService {
	return &addonRegistryWebService{
		addonUsecase: u,
	}
}

type addonRegistryWebService struct {
	addonUsecase usecase.AddonUsecase
}

func (s *addonRegistryWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path("/v1/addon_registries").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for addon registry management")

	tags := []string{"addon_registry"}

	// Create
	ws.Route(ws.POST("/").To(s.createAddonRegistry).
		Doc("create an addon registry").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateAddonRegistryRequest{}).
		Returns(200, "", apis.AddonRegistryMeta{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.AddonRegistryMeta{}))

	// Delete
	ws.Route(ws.DELETE("/{name}").To(s.deleteAddonRegistry).
		Doc("delete an addon registry").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon registry").DataType("string")).
		Returns(200, "", apis.AddonRegistryMeta{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.AddonRegistryMeta{}))

	return ws
}

func (s *addonRegistryWebService) createAddonRegistry(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	meta, err := s.addonUsecase.CreateAddonRegistry(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create addon registry failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(meta); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonRegistryWebService) deleteAddonRegistry(req *restful.Request, res *restful.Response) {
	r, err := s.addonUsecase.GetAddonRegistryModel(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(*utils.ConvertAddonRegistryModel2AddonRegistryMeta(r)); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
