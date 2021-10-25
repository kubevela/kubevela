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
	"text/template"

	"github.com/Masterminds/sprig"
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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
	ws.Path(versionPrefix+"/addons").
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
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DetailAddonResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Param(ws.QueryParameter("name", "addon name to query detail").DataType("string").Required(true)).
		Writes(apis.DetailAddonResponse{}))

	// GET status
	ws.Route(ws.GET("/status").To(s.statusAddon).
		Doc("show status of an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Param(ws.QueryParameter("name", "addon name to query status").DataType("string").Required(true)).
		Writes(apis.AddonStatusResponse{}))

	// enable addon
	ws.Route(ws.POST("/enable").To(s.enableAddon).
		Doc("enable an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Param(ws.QueryParameter("name", "addon name to enable").DataType("string").Required(true)).
		Writes(apis.AddonStatusResponse{}))

	// disable addon
	ws.Route(ws.POST("/disable").To(s.disableAddon).
		Doc("disable an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.AddonStatusResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Param(ws.QueryParameter("name", "addon name to enable").DataType("string").Required(true)).
		Writes(apis.AddonStatusResponse{}))

	return ws
}

func (s *addonWebService) listAddons(req *restful.Request, res *restful.Response) {
	detailAddons, err := s.addonUsecase.ListAddons(req.Request.Context(), false)
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

func (s *addonWebService) detailAddon(req *restful.Request, res *restful.Response) {
	name := req.QueryParameter("name")
	addon, err := s.addonUsecase.GetAddon(req.Request.Context(), name)
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
	if err = validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	name := req.QueryParameter("name")
	addon, err := s.addonUsecase.GetAddon(req.Request.Context(), name)
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
	name := req.QueryParameter("name")
	addon, err := s.addonUsecase.GetAddon(req.Request.Context(), name)
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
	name := req.QueryParameter("name")
	status, err := s.addonUsecase.StatusAddon(name)
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
	ghClt, _ := clientArgs.GetClient()
	applicator := apply.NewAPIApplicator(ghClt)
	err = applicator.Apply(context.TODO(), app)
	if err != nil {
		log.Logger.Errorf("apply application fail: %s", err.Error())
		return bcode.ErrAddonApplyFail
	}
	return nil
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
