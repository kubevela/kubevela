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

type definitionWebservice struct {
	definitionUsecase usecase.DefinitionUsecase
}

func (d *definitionWebservice) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/definitions").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for definition manage")

	tags := []string{"definition"}

	ws.Route(ws.GET("/").To(d.listDefinitions).
		Doc("list all definitions").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("type", "query the definition type").DataType("string").Required(true).AllowableValues(map[string]string{"component": "", "trait": "", "workflowstep": ""})).
		Param(ws.QueryParameter("envName", "if specified, query the definition supported by the env.").DataType("string")).
		Param(ws.QueryParameter("appliedWorkload", "if specified, query the trait definition applied to the workload.").DataType("string")).
		Returns(200, "OK", apis.ListDefinitionResponse{}).
		Writes(apis.ListDefinitionResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}").To(d.detailDefinition).
		Doc("detail definition").
		Param(ws.PathParameter("name", "identifier of the definition").DataType("string")).
		Param(ws.QueryParameter("type", "query the definition type").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "create success", apis.DetailDefinitionResponse{}).
		Writes(apis.DetailDefinitionResponse{}).Do(returns200, returns500))

	ws.Filter(authCheckFilter)
	return ws
}

// NewDefinitionWebservice new definition webservice
func NewDefinitionWebservice(du usecase.DefinitionUsecase) WebService {
	return &definitionWebservice{
		definitionUsecase: du,
	}
}

func (d *definitionWebservice) listDefinitions(req *restful.Request, res *restful.Response) {
	definitions, err := d.definitionUsecase.ListDefinitions(req.Request.Context(), req.QueryParameter("envName"), req.QueryParameter("type"), req.QueryParameter("appliedWorkload"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListDefinitionResponse{Definitions: definitions}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (d *definitionWebservice) detailDefinition(req *restful.Request, res *restful.Response) {
	definition, err := d.definitionUsecase.DetailDefinition(req.Request.Context(), req.PathParameter("name"), req.QueryParameter("type"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(definition); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
