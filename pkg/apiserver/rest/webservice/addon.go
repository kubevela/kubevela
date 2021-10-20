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
	"bytes"
	"context"
	"net/url"
	"path"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig"
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/cli"
	"github.com/oam-dev/kubevela/references/plugins"
)

const (
	// AddonFileName is the addon file name
	AddonFileName string = "addon.yaml"
	// AddonReadmeFileName is the addon readme file name
	AddonReadmeFileName string = "readme.md"
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

	// Backward compatibility with ConfigMap addons.
	// We will deprecate ConfigMap and use Git based registry.
	addons, err := getAddonsFromConfigMap()
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	for _, r := range rs {
		getAddons, err := getAddonsFromGit(r.Git.URL, r.Git.Dir)
		if err != nil {
			bcode.ReturnError(req, res, err)
			return
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

	err = s.applyAddonData(addon.DeployData, createReq)
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

func (s *addonWebService) applyAddonData(data string, request apis.EnableAddonRequest) error {
	t, err := template.New("addon-template").Delims("[[", "]]").Funcs(sprig.TxtFuncMap()).Parse(data)
	if err != nil {
		return bcode.ErrAddonRenderFail
	}
	buf := bytes.Buffer{}
	err = t.Execute(&buf, request)
	if err != nil {
		return bcode.ErrAddonRenderFail
	}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, _, err = dec.Decode(buf.Bytes(), nil, obj)
	if err != nil {
		return bcode.ErrAddonRenderFail
	}
	clientArgs, _ := common.InitBaseRestConfig()
	clt, _ := clientArgs.GetClient()
	applicator := apply.NewAPIApplicator(clt)
	err = applicator.Apply(context.TODO(), obj)
	if err != nil {
		return bcode.ErrAddonApplyFail
	}
	return nil
}

func (s *addonWebService) checkAddonStatus(name string) (*apis.AddonStatusResponse, error) {
	panic("")
}

func (s *addonWebService) deleteAddonData(data string) error {
	panic("")
}

func getAddonsFromGit(baseUrl, dir string) ([]*apis.AddonMeta, error) {
	metas := []*apis.AddonMeta{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	client := github.NewClient(nil)
	// TODO add error handling
	baseUrl = strings.TrimSuffix(baseUrl, ".git")
	u, err := url.Parse(baseUrl)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, dir)
	_, content, err := plugins.Parse(u.String())
	if err != nil {
		return nil, err
	}
	_, dirs, _, err := client.Repositories.GetContents(context.Background(), content.Owner, content.Repo, content.Path, nil)
	if err != nil {
		return nil, err
	}
	for _, subItems := range dirs {
		if *subItems.Type == "file" {
			continue
		}
		meta := apis.AddonMeta{
			Name: *subItems.Name,
		}
		var err error
		_, files, _, err := client.Repositories.GetContents(context.Background(), content.Owner, content.Repo, *subItems.Path, nil)
		// get addon.yaml
		for _, file := range files {
			if *file.Name != AddonFileName {
				continue
			}
			addonContent, _, _, err := client.Repositories.GetContents(context.Background(), content.Owner, content.Repo, *file.Path, nil)
			if err != nil {
				break
			}
			addonStr, _ := addonContent.GetContent()
			obj := &unstructured.Unstructured{}
			_, _, err = dec.Decode([]byte(addonStr), nil, obj)
			if err != nil {
				break
			}
			meta.Description = obj.GetAnnotations()[cli.DescAnnotation]
			break
		}
		if err != nil {
			continue
		}
		metas = append(metas, &meta)
	}
	return metas, nil
}

func getAddonsFromConfigMap() ([]*apis.AddonMeta, error) {
	repo, err := cli.NewAddonRepo()
	if err != nil {
		return nil, errors.Wrap(err, "get configMap addon repo err")
	}
	addons := repo.ListAddons()
	metas := []*apis.AddonMeta{}
	for _, addon := range addons {
		metas = append(metas, &apis.AddonMeta{
			Name: addon.Name,
			// TODO add actual Version, Icon, tags
			Version:     "v1alpha1",
			Description: addon.Description,
			Icon:        "",
			Tags:        nil,
		})
	}
	return metas, nil

}
