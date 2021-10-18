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
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// NewAddonWebService returns addon web service
func NewAddonWebService(u usecase.AddonUsecase) WebService {
	return &addonWebService{
		addonUsecase: u,
	}
}

type addonWebService struct {
	addonUsecase usecase.AddonUsecase
}

func (s *addonWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path("/v1/addons").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for addon management")

	tags := []string{"addon"}

	// List
	ws.Route(ws.GET("/").To(s.listAddons).
		Doc("list all addons").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ListAddonResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ListAddonResponse{}))

	// GET
	ws.Route(ws.GET("/{name}").To(s.detailAddon).
		Doc("show details of an addon").
		Filter(s.addonCheckFilter).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon").DataType("string")).
		Returns(200, "", apis.DetailAddonResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailAddonResponse{}))

	// GET status
	ws.Route(ws.GET("/{name}/status").To(s.statusAddon).
		Doc("show status of an addon").
		Filter(s.addonCheckFilter).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon").DataType("string")).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.AddonStatusResponse{}))

	// enable addon
	ws.Route(ws.POST("/{name}/enable").To(s.enableAddon).
		Doc("enable an addon").
		Filter(s.addonCheckFilter).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.EnableAddonRequest{}).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.AddonStatusResponse{}))

	// disable addon
	ws.Route(ws.POST("/{name}/disable").To(s.disableAddon).
		Doc("disable an addon").
		Filter(s.addonCheckFilter).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	return ws
}

func (s *addonWebService) listAddons(req *restful.Request, res *restful.Response) {
	rs, err := s.addonUsecase.ListAddonRegistries(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	var addons []*apis.AddonMeta
	for _, r := range rs {
		var getAddons []*apis.AddonMeta
		switch {
		case r.ConfigMap != nil:
			getAddons = getAddonsFromConfigMap(r.ConfigMap.Name, r.ConfigMap.Namespace)
		case r.Git != nil:
			getAddons = getAddonsFromGit(r.Git.URL, r.Git.Dir)
		}

		addons = append(addons, getAddons...)
	}

	err = res.WriteEntity(apis.ListAddonResponse{Addons: addons})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonWebService) detailAddon(req *restful.Request, res *restful.Response) {
	addon := req.Request.Context().Value(&apis.CtxKeyAddon).(*model.Addon)
	detail, err := s.addonUsecase.DetailAddon(req.Request.Context(), addon)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	err = res.WriteEntity(detail)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonWebService) addonCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	addon, err := s.addonUsecase.GetAddonModel(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyAddon, addon))
	chain.ProcessFilter(req, res)
}

func (s *addonWebService) enableAddon(req *restful.Request, res *restful.Response) {
	addon := req.Request.Context().Value(&apis.CtxKeyAddon).(*model.Addon)

	var createReq apis.EnableAddonRequest
	err := req.ReadEntity(&createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	err = s.applyAddonData(addon.DeployData, createReq.Envs)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	s.statusAddon(req, res)
}

func (s *addonWebService) disableAddon(req *restful.Request, res *restful.Response) {
	addon := req.Request.Context().Value(&apis.CtxKeyAddon).(*model.Addon)
	err := s.deleteAddonData(addon.DeployData)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	s.statusAddon(req, res)
}

func (s *addonWebService) statusAddon(req *restful.Request, res *restful.Response) {
	addon := req.Request.Context().Value(&apis.CtxKeyAddon).(*model.Addon)

	status, err := s.checkAddonStatus(addon.Name)
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

func (s *addonWebService) applyAddonData(data string, envs map[string]string) error {
	panic("")
}

func (s *addonWebService) checkAddonStatus(name string) (*apis.AddonStatusResponse, error) {
	panic("")
}

func (s *addonWebService) deleteAddonData(data string) error {
	panic("")
}

func getAddonsFromGit(url, dir string) []*apis.AddonMeta {
	panic("")
}

func getAddonsFromConfigMap(name, namespace string) []*apis.AddonMeta {
	panic("")
}
