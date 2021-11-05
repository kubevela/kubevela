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
	restful "github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type namespaceWebService struct {
	namespaceUsecase usecase.NamespaceUsecase
}

// NewNamespaceWebService new namespace webservice
func NewNamespaceWebService(namespaceUsecase usecase.NamespaceUsecase) WebService {
	return &namespaceWebService{namespaceUsecase: namespaceUsecase}
}

func (n *namespaceWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/namespaces").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for namespace manage")

	tags := []string{"namespace"}

	ws.Route(ws.GET("/").To(n.listNamespaces).
		Doc("list all namespaces").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ListNamespaceResponse{}).
		Writes(apis.ListNamespaceResponse{}))

	ws.Route(ws.POST("/").To(n.createNamespace).
		Doc("create namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateNamespaceRequest{}).
		Returns(200, "", apis.NamespaceDetailResponse{}).
		Writes(apis.NamespaceDetailResponse{}))
	return ws
}

func (n *namespaceWebService) listNamespaces(req *restful.Request, res *restful.Response) {
	namespaces, err := n.namespaceUsecase.ListNamespaces(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListNamespaceResponse{Namespaces: namespaces}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *namespaceWebService) createNamespace(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateNamespaceRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the usecase layer code
	namespaceBase, err := n.namespaceUsecase.CreateNamespace(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create application failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(apis.NamespaceDetailResponse{NamespaceBase: *namespaceBase}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
