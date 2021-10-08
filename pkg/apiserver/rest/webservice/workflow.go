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

type workflowWebService struct {
}

func (c *workflowWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/workflows").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for cluster manage")

	tags := []string{"cluster"}

	ws.Route(ws.GET("/{name}").To(noop).
		Doc("detail application workflow").
		Param(ws.PathParameter("name", "identifier of the workflow, Currently, the application name is used.").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{name}").To(noop).
		Doc("create or update application workflow config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Reads(apis.UpdateWorkflowRequest{}).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}/records").To(noop).
		Doc("query application workflow execution record").
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("page", "Query the page number.").DataType("integer")).
		Param(ws.PathParameter("pageSize", "Query the page size number.").DataType("integer")).
		Writes(apis.ListWorkflowRecordsResponse{}).Do(returns200, returns500))

	return ws
}
