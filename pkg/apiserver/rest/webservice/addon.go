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

	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

type addonWebService struct {
}

func (c *addonWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path("/v1/addons").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for addon management")

	tags := []string{"addon"}

	// List
	ws.Route(ws.GET("/").To(noop).
		Doc("list all addons").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("cluster", "Cluster-based search").DataType("string")).
		Writes(apis.ListAddonResponse{}).Do(returns200, returns500))

	// Create
	ws.Route(ws.POST("/").To(noop).
		Doc("create an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateAddonRequest{}).
		Writes(apis.AddonMeta{}))

	// Delete
	ws.Route(ws.DELETE("/{name}").To(noop).
		Doc("delete an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon").DataType("string")).
		Writes(apis.AddonMeta{}))

	// GET
	ws.Route(ws.GET("/{name}").To(noop).
		Doc("show details of an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon").DataType("string")).
		Writes(apis.DetailAddonResponse{}))

	// GET status
	ws.Route(ws.GET("/{name}/status").To(noop).
		Doc("show status of an addon").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the addon").DataType("string")).
		Writes(apis.AddonStatusResponse{}))

	// vela enable addon
	ws.Route(ws.POST("/{name}/enable").To(noop).
		Doc("enable an addon on a cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("cluster", "cluster name").DataType("string")).
		Writes(apis.AddonMeta{}))

	// vela disable addon
	ws.Route(ws.POST("/{name}/disable").To(noop).
		Doc("disable an addon on a cluster").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("cluster", "cluster name").DataType("string")).
		Writes(apis.AddonMeta{}))

	return ws
}
