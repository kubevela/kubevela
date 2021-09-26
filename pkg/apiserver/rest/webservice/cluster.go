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
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type clusterWebService struct {
	clusterUsecase usecase.ClusterUsecase
}

func (c *clusterWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrifix+"/clusters").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for cluster manage")

	tags := []string{"cluster"}

	ws.Route(ws.GET("/").To(noop).
		Doc("list all clusters").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Writes(apis.ListClusterResponse{}).Do(returns200, returns500))

	ws.Route(ws.POST("/").To(c.createKubeCluster).
		Doc("create cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(&apis.CreateClusterRequest{}).
		Writes(apis.ClusterBase{}))

	ws.Route(ws.GET("/{clusterName}").To(noop).
		Doc("detail cluster info").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("clusterName", "identifier of the cluster").DataType("string")).
		Writes(apis.DetailClusterResponse{}))

	ws.Route(ws.GET("/{clusterName}/addons").To(noop).
		Doc("list cluster addons info").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("clusterName", "identifier of the cluster").DataType("string")).
		Writes(apis.ListClusterAddonResponse{}))

	ws.Route(ws.POST("/{clusterName}/addons").To(noop).
		Doc("add addon for the cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("clusterName", "identifier of the cluster").DataType("string")).
		Writes(apis.DeatilClusterAddonResponse{}).Returns(200, "", apis.DeatilClusterAddonResponse{}))
	return ws
}

func (c *clusterWebService) createKubeCluster(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateClusterRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the usecase layer code
	clusterBase, err := c.clusterUsecase.CreateKubeCluster(req.Request.Context(), createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(clusterBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
