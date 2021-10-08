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

type applicationWebService struct {
}

func (c *applicationWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/applications").
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
		Writes(apis.ListApplicationResponse{}))

	ws.Route(ws.POST("/").To(noop).
		Doc("create one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateApplicationRequest{}).
		Writes(apis.ApplicationBase{}))

	ws.Route(ws.DELETE("/{name}").To(noop).
		Doc("delete one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Writes(apis.ApplicationBase{}))

	ws.Route(ws.GET("/{name}").To(noop).
		Doc("detail one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Writes(apis.DetailApplicationResponse{}))

	ws.Route(ws.POST("/{name}/template").To(noop).
		Doc("create one application template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Reads(apis.CreateApplicationTemplateRequest{}).
		Writes(apis.ApplicationTemplateBase{}))

	ws.Route(ws.POST("/{name}/deploy").To(noop).
		Doc("deploy or update the application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Writes(apis.ApplicationBase{}))

	ws.Route(ws.GET("/{name}/components").To(noop).
		Doc("gets the component topology of the application").
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("cluster", "list components that deployed in define cluster").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(apis.ComponentListResponse{}))

	ws.Route(ws.POST("/{name}/components").To(noop).
		Doc("create component for application").
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateComponentRequest{}).
		Writes(apis.ComponentBase{}))

	ws.Route(ws.POST("/{name}/policies").To(noop).
		Doc("create policy for application").
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreatePolicyRequest{}).
		Writes(apis.DetailPolicyResponse{}))

	ws.Route(ws.GET("/{name}/policies/{policyName}").To(noop).
		Doc("detail policy for application").
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(apis.DetailPolicyResponse{}))

	ws.Route(ws.DELETE("/{name}/policies/{policyName}").To(noop).
		Doc("detail policy for application").
		Param(ws.PathParameter("name", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(apis.DetailPolicyResponse{}))
	return ws
}
