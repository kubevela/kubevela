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
)

type namespaceWebService struct {
}

func (c *namespaceWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/namespaces").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for namespace manage")

	tags := []string{"namespace"}

	ws.Route(ws.GET("/").To(noop).
		Doc("list all namespaces").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(apis.ListNamespaceResponse{}))

	ws.Route(ws.POST("/").To(noop).
		Doc("create namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateNamespaceRequest{}).
		Writes(apis.NamesapceDetailResponse{}))

	ws.Route(ws.GET("/{namespace}").To(noop).
		Doc("get one namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("namespace", "identifier of the namespace").DataType("string")).
		Writes(apis.NamesapceDetailResponse{}))

	// Compatible with historical apis
	ws.Route(ws.GET("/{namespace}/applications/:appname").To(noop).
		Doc("get the specified oam application in the specified namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("namespace", "identifier of the namespace").DataType("string")).
		Param(ws.PathParameter("appname", "identifier of the oam application").DataType("string")).
		Writes(apis.ApplicationResponse{}))

	ws.Route(ws.POST("/{namespace}/applications/:appname").To(noop).
		Doc("create or update oam application in the specified namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("namespace", "identifier of the namespace").DataType("string")).
		Param(ws.PathParameter("appname", "identifier of the oam application").DataType("string")).
		Reads(apis.ApplicationRequest{}))

	ws.Route(ws.DELETE("/{namespace}/applications/:appname").To(noop).
		Doc("create or update oam application in the specified namespace").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("namespace", "identifier of the namespace").DataType("string")).
		Param(ws.PathParameter("appname", "identifier of the oam application").DataType("string")))
	return ws
}
