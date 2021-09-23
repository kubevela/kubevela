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

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1beta1"
)

type applicationWebService struct {
}

func (c *applicationWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrifix+"/applications").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for application manage")

	tags := []string{"application"}

	ws.Route(ws.GET("/").To(noop).
		Doc("list all applications").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Param(ws.QueryParameter("namespace", "Namespace-based search").DataType("string")).
		Param(ws.QueryParameter("cluster", "Cluster-based search").DataType("string")).
		Writes(v1beta1.ListApplicationResponse{}))

	ws.Route(ws.POST("/").To(noop).
		Doc("create one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(v1beta1.CreateApplicationRequest{}).
		Writes(v1beta1.ApplicationBase{}))

	ws.Route(ws.DELETE("/{name}").To(noop).
		Doc("delete one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(v1beta1.ApplicationBase{}))

	ws.Route(ws.GET("/{name}").To(noop).
		Doc("detail one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(v1beta1.DetailApplicationResponse{}))

	ws.Route(ws.POST("/{name}/template").To(noop).
		Doc("create one application template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(v1beta1.ApplicationTemplateBase{}))

	ws.Route(ws.POST("/{name}/deploy").To(noop).
		Doc("deploy or update the application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(v1beta1.ApplicationBase{}))

	ws.Route(ws.GET("/{name}/components").To(noop).
		Doc("gets the component topology of the application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(v1beta1.ComponentListResponse{}))

	ws.Route(ws.POST("/{name}/components").To(noop).
		Doc("create component for application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(v1beta1.CreateComponentRequest{}).
		Writes(v1beta1.ComponentBase{}))
	return ws
}
