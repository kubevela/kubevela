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
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// ClusterWebService cluster manage webservice
type ClusterWebService struct {
	clusterUsecase usecase.ClusterUsecase
}

// NewClusterWebService new cluster webservice
func NewClusterWebService(clusterUsecase usecase.ClusterUsecase) *ClusterWebService {
	return &ClusterWebService{clusterUsecase: clusterUsecase}
}

// GetWebService -
func (c *ClusterWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/clusters").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for cluster manage")

	tags := []string{"cluster"}

	ws.Route(ws.GET("/").To(c.listKubeClusters).
		Doc("list all clusters").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Param(ws.QueryParameter("page", "Page for paging").DataType("integer").DefaultValue("0")).
		Param(ws.QueryParameter("pageSize", "PageSize for paging").DataType("integer").DefaultValue("20")).
		Returns(200, "OK", apis.ListClusterResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListClusterResponse{}).Do(returns200, returns500))

	ws.Route(ws.POST("/").To(c.createKubeCluster).
		Doc("create cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateClusterRequest{}).
		Returns(200, "OK", apis.ClusterBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ClusterBase{}))

	ws.Route(ws.GET("/{clusterName}").To(c.getKubeCluster).
		Doc("detail cluster info").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("clusterName", "identifier of the cluster").DataType("string")).
		Returns(200, "OK", apis.DetailClusterResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailClusterResponse{}))

	ws.Route(ws.PUT("/{clusterName}").To(c.modifyKubeCluster).
		Doc("modify cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("clusterName", "identifier of the cluster").DataType("string")).
		Reads(apis.CreateClusterRequest{}).
		Returns(200, "OK", apis.ClusterBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ClusterBase{}))

	ws.Route(ws.DELETE("/{clusterName}").To(c.deleteKubeCluster).
		Doc("delete cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("clusterName", "identifier of the cluster").DataType("string")).
		Returns(200, "OK", apis.ClusterBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ClusterBase{}))

	ws.Route(ws.POST("/{clusterName}/namespaces").To(c.createNamespace).
		Doc("create namespace in cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("clusterName", "name of the target cluster").DataType("string")).
		Reads(apis.CreateClusterNamespaceRequest{}).
		Returns(200, "OK", apis.CreateClusterNamespaceResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.CreateClusterNamespaceResponse{}))

	ws.Route(ws.POST("/cloud_clusters/{provider}").To(c.listCloudClusters).
		Doc("list cloud clusters").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("provider", "identifier of the cloud provider").DataType("string")).
		Param(ws.QueryParameter("page", "Page for paging").DataType("integer").DefaultValue("0")).
		Param(ws.QueryParameter("pageSize", "PageSize for paging").DataType("integer").DefaultValue("20")).
		Reads(apis.AccessKeyRequest{}).
		Returns(200, "OK", apis.ListCloudClusterResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListCloudClusterResponse{}))

	ws.Route(ws.POST("/cloud_clusters/{provider}/connect").To(c.connectCloudCluster).
		Doc("create cluster from cloud cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("provider", "identifier of the cloud provider").DataType("string")).
		Reads(apis.ConnectCloudClusterRequest{}).
		Returns(200, "OK", apis.ClusterBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ClusterBase{}))

	ws.Route(ws.POST("/cloud_clusters/{provider}/create").To(c.createCloudCluster).
		Doc("create cloud cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("provider", "identifier of the cloud provider").DataType("string").Required(true)).
		Reads(apis.CreateCloudClusterRequest{}).
		Returns(200, "OK", apis.CreateCloudClusterResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.CreateCloudClusterResponse{}))

	ws.Route(ws.GET("/cloud_clusters/{provider}/creation/{cloudClusterName}").To(c.getCloudClusterCreationStatus).
		Doc("check cloud cluster create status").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("provider", "identifier of the cloud provider").DataType("string")).
		Param(ws.PathParameter("cloudClusterName", "identifier for cloud cluster which is creating").DataType("string")).
		Returns(200, "OK", apis.CreateCloudClusterResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.CreateCloudClusterResponse{}))

	ws.Route(ws.GET("/cloud_clusters/{provider}/creation").To(c.listCloudClusterCreation).
		Doc("list cloud cluster creation").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("provider", "identifier of the cloud provider").DataType("string")).
		Returns(200, "OK", apis.ListCloudClusterCreationResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListCloudClusterCreationResponse{}))

	ws.Route(ws.DELETE("/cloud_clusters/{provider}/creation/{cloudClusterName}").To(c.deleteCloudClusterCreation).
		Doc("delete cloud cluster creation").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("provider", "identifier of the cloud provider").DataType("string")).
		Param(ws.PathParameter("cloudClusterName", "identifier for cloud cluster which is creating").DataType("string")).
		Returns(200, "OK", apis.CreateCloudClusterResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.CreateCloudClusterResponse{}))

	return ws
}

func (c *ClusterWebService) listKubeClusters(req *restful.Request, res *restful.Response) {
	query := req.QueryParameter("query")
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Call the usecase layer code
	clusters, err := c.clusterUsecase.ListKubeClusters(req.Request.Context(), query, page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(clusters); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) createKubeCluster(req *restful.Request, res *restful.Response) {
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

func (c *ClusterWebService) getKubeCluster(req *restful.Request, res *restful.Response) {
	clusterName := req.PathParameter("clusterName")

	// Call the usecase layer code
	clusterDetail, err := c.clusterUsecase.GetKubeCluster(req.Request.Context(), clusterName)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(clusterDetail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) modifyKubeCluster(req *restful.Request, res *restful.Response) {
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
	clusterName := req.PathParameter("clusterName")

	// Call the usecase layer code
	clusterBase, err := c.clusterUsecase.ModifyKubeCluster(req.Request.Context(), createReq, clusterName)
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

func (c *ClusterWebService) deleteKubeCluster(req *restful.Request, res *restful.Response) {
	clusterName := req.PathParameter("clusterName")

	// Call the usecase layer code
	clusterBase, err := c.clusterUsecase.DeleteKubeCluster(req.Request.Context(), clusterName)
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

func (c *ClusterWebService) createNamespace(req *restful.Request, res *restful.Response) {
	clusterName := req.PathParameter("clusterName")

	// Verify the validity of parameters
	var createReq apis.CreateClusterNamespaceRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Call the usecase layer code
	resp, err := c.clusterUsecase.CreateClusterNamespace(req.Request.Context(), clusterName, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) listCloudClusters(req *restful.Request, res *restful.Response) {
	provider := req.PathParameter("provider")
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Verify the validity of parameters
	var accessKeyRequest apis.AccessKeyRequest
	if err := req.ReadEntity(&accessKeyRequest); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&accessKeyRequest); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Call the usecase layer code
	clustersResp, err := c.clusterUsecase.ListCloudClusters(req.Request.Context(), provider, accessKeyRequest, page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(clustersResp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) connectCloudCluster(req *restful.Request, res *restful.Response) {
	provider := req.PathParameter("provider")

	// Verify the validity of parameters
	var connectReq apis.ConnectCloudClusterRequest
	if err := req.ReadEntity(&connectReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&connectReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Call the usecase layer code
	cluster, err := c.clusterUsecase.ConnectCloudCluster(req.Request.Context(), provider, connectReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(cluster); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) createCloudCluster(req *restful.Request, res *restful.Response) {
	provider := req.PathParameter("provider")

	// Verify the validity of parameters
	var createReq apis.CreateCloudClusterRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Call the usecase layer code
	resp, err := c.clusterUsecase.CreateCloudCluster(req.Request.Context(), provider, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) getCloudClusterCreationStatus(req *restful.Request, res *restful.Response) {
	provider := req.PathParameter("provider")
	cloudClusterName := req.PathParameter("cloudClusterName")

	// Call the usecase layer code
	resp, err := c.clusterUsecase.GetCloudClusterCreationStatus(req.Request.Context(), provider, cloudClusterName)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) listCloudClusterCreation(req *restful.Request, res *restful.Response) {
	provider := req.PathParameter("provider")

	// Call the usecase layer code
	resp, err := c.clusterUsecase.ListCloudClusterCreation(req.Request.Context(), provider)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *ClusterWebService) deleteCloudClusterCreation(req *restful.Request, res *restful.Response) {
	provider := req.PathParameter("provider")
	cloudClusterName := req.PathParameter("cloudClusterName")

	// Call the usecase layer code
	resp, err := c.clusterUsecase.DeleteCloudClusterCreation(req.Request.Context(), provider, cloudClusterName)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
