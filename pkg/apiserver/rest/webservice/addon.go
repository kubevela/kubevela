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

	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"

	"github.com/Masterminds/sprig"
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"

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
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon").DataType("string")).
		Returns(200, "", apis.DetailAddonResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailAddonResponse{}))

	// GET status
	ws.Route(ws.GET("/{name}/status").To(s.statusAddon).
		Doc("show status of an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon").DataType("string")).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.AddonStatusResponse{}))

	// enable addon
	ws.Route(ws.POST("/{name}/enable").To(s.enableAddon).
		Doc("enable an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.EnableAddonRequest{}).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.AddonStatusResponse{}))

	// disable addon
	ws.Route(ws.POST("/{name}/disable").To(s.disableAddon).
		Doc("disable an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	return ws
}

func (s *addonWebService) listAddons(req *restful.Request, res *restful.Response) {
	detailAddons, err := s.getAllAddons(req.Request.Context(), false)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	var addons []*apis.AddonMeta

	for _, d := range detailAddons {
		addons = append(addons, &d.AddonMeta)
	}

	err = res.WriteEntity(apis.ListAddonResponse{Addons: addons})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (s *addonWebService) getAllAddons(ctx context.Context, detailed bool) ([]*apis.DetailAddonResponse, error) {
	// Backward compatibility with ConfigMap addons.
	// We will deprecate ConfigMap and use Git based registry.
	addons, err := getAddonsFromConfigMap(detailed)
	if err != nil {
		return nil, err
	}

	rs, err := s.addonUsecase.ListAddonRegistries(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		getAddons, err := getAddonsFromGit(r.Git.URL, r.Git.Dir, detailed)
		if err != nil {
			return nil, err
		}
		addons = mergeAddons(addons, getAddons)
	}
	return addons, nil
}

func (s *addonWebService) detailAddon(req *restful.Request, res *restful.Response) {
	name := req.PathParameter("name")
	addon, err := s.getAddon(req.Request.Context(), name)
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
	err := req.ReadEntity(&createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	name := req.PathParameter("name")
	addon, err := s.getAddon(req.Request.Context(), name)
	if err != nil {
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
	name := req.PathParameter("name")
	addon, err := s.getAddon(req.Request.Context(), name)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = s.deleteAddonData(addon.DeployData)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	s.statusAddon(req, res)
}

func (s *addonWebService) statusAddon(req *restful.Request, res *restful.Response) {
	name := req.PathParameter("name")
	status, err := s.checkAddonStatus(name)
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

// renderAddonApp can render string to unstructured, args can be nil
func renderAddonApp(data string, args *apis.EnableAddonRequest) (*unstructured.Unstructured, error) {
	if args == nil {
		args = &apis.EnableAddonRequest{Args: map[string]string{}}
	}

	t, err := template.New("addon-template").Delims("[[", "]]").Funcs(sprig.TxtFuncMap()).Parse(data)
	if err != nil {
		return nil, bcode.ErrAddonRenderFail
	}
	buf := bytes.Buffer{}
	err = t.Execute(&buf, args)
	if err != nil {
		return nil, bcode.ErrAddonRenderFail
	}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, _, err = dec.Decode(buf.Bytes(), nil, obj)
	if err != nil {
		return nil, bcode.ErrAddonRenderFail
	}
	return obj, nil
}

func (s *addonWebService) applyAddonData(data string, request apis.EnableAddonRequest) error {
	app, err := renderAddonApp(data, &request)
	if err != nil {
		return err
	}
	clientArgs, _ := common.InitBaseRestConfig()
	clt, _ := clientArgs.GetClient()
	applicator := apply.NewAPIApplicator(clt)
	err = applicator.Apply(context.TODO(), app)
	if err != nil {
		return bcode.ErrAddonApplyFail
	}
	return nil
}

func (s *addonWebService) checkAddonStatus(name string) (*apis.AddonStatusResponse, error) {
	// TODO fix this， addons can be got from not only configMap
	addons, err := getAddonsFromConfigMap(false)
	if err != nil {
		return nil, bcode.ErrGetConfigMapAddonFail
	}
	var exist bool
	for _, addon := range addons {
		if addon.Name == name {
			exist = true
		}
	}
	if !exist {
		return nil, bcode.ErrAddonNotExist
	}
	args, err := common.InitBaseRestConfig()
	if err != nil {
		return nil, bcode.ErrGetClientFail
	}
	clt, err := args.GetClient()
	if err != nil {
		return nil, bcode.ErrGetClientFail
	}
	var app v1beta1.Application
	err = clt.Get(context.Background(), client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      name,
	}, &app)
	if err != nil {
		if errors2.IsNotFound(err) {
			return &apis.AddonStatusResponse{
				Phase:            apis.AddonPhaseDisabled,
				EnablingProgress: nil,
			}, nil
		}
		return nil, bcode.ErrGetApplicationFail
	}
	// TODO we don't know when addon is disabling
	switch app.Status.Phase {
	case common2.ApplicationRunning:
		return &apis.AddonStatusResponse{
			Phase:            apis.AddonPhaseEnabled,
			EnablingProgress: nil,
		}, nil
	default:
		return &apis.AddonStatusResponse{
			Phase:            apis.AddonPhaseEnabling,
			EnablingProgress: nil,
		}, nil
	}
}

func (s *addonWebService) deleteAddonData(data string) error {
	app, err := renderAddonApp(data, nil)
	if err != nil {
		return err
	}
	args, err := common.InitBaseRestConfig()
	if err != nil {
		return bcode.ErrGetClientFail
	}
	clt, err := args.GetClient()
	if err != nil {
		return bcode.ErrGetClientFail
	}
	err = clt.Get(context.Background(), client.ObjectKey{
		Namespace: app.GetNamespace(),
		Name:      app.GetName(),
	}, app)
	if err != nil {
		return bcode.ErrAddonNotEnabled
	}
	err = clt.Delete(context.Background(), app)
	if err != nil {
		return bcode.ErrAddonDisableFail
	}
	return nil

}

func (s *addonWebService) getAddon(ctx context.Context, name string) (*apis.DetailAddonResponse, error) {
	addons, err := s.getAllAddons(ctx, true)
	if err != nil {
		return nil, err
	}

	for _, addon := range addons {
		if addon.Name == name {
			return addon, nil
		}
	}
	return nil, bcode.ErrAddonNotExist
}

func mergeAddons(a1, a2 []*apis.DetailAddonResponse) []*apis.DetailAddonResponse {
	for _, item := range a2 {
		if hasAddon(a1, item.Name) {
			continue
		}
		a1 = append(a1, item)
	}
	return a1
}

func hasAddon(addons []*apis.DetailAddonResponse, name string) bool {
	for _, addon := range addons {
		if addon.Name == name {
			return true
		}
	}
	return false
}

func getAddonsFromGit(baseUrl, dir string, detailed bool) ([]*apis.DetailAddonResponse, error) {
	addons := []*apis.DetailAddonResponse{}
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
		addons = append(addons, &apis.DetailAddonResponse{
			AddonMeta: meta,
		})
	}
	return addons, nil
}

func getAddonsFromConfigMap(detailed bool) ([]*apis.DetailAddonResponse, error) {
	repo, err := cli.NewAddonRepo()
	if err != nil {
		return nil, errors.Wrap(err, "get configMap addon repo err")
	}
	cliAddons := repo.ListAddons()
	addons := []*apis.DetailAddonResponse{}
	for _, addon := range cliAddons {
		d := &apis.DetailAddonResponse{
			AddonMeta: apis.AddonMeta{
				Name: addon.Name,
				// TODO add actual Version, Icon, tags
				Version:     "v1alpha1",
				Description: addon.Description,
				Icon:        "",
				Tags:        nil,
			},
		}
		addons = append(addons, d)
	}
	return addons, nil

}
