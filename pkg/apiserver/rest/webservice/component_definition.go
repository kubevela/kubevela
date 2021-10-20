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

type componentDefinitionWebservice struct {
	definitionUsecase usecase.DefinitionUsecase
}

func (c *componentDefinitionWebservice) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/componentdefinitions").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for componentdefinition manage")

	tags := []string{"componentdefinition"}

	ws.Route(ws.GET("/").To(c.listComponentDefinition).
		Doc("list all componentdefinition").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("envName", "if specified, query the componentdefinition supported by the env.").DataType("string")).
		Returns(200, "", apis.ListComponentDefinitionResponse{}).
		Writes(apis.ListComponentDefinitionResponse{}))
	return ws
}

// NewComponentDefinitionWebservice new componentdefinition webservice
func NewComponentDefinitionWebservice(du usecase.DefinitionUsecase) WebService {
	return &componentDefinitionWebservice{
		definitionUsecase: du,
	}
}

func (c *componentDefinitionWebservice) listComponentDefinition(req *restful.Request, res *restful.Response) {
	componentDefinitions, err := c.definitionUsecase.ListComponentDefinitions(req.Request.Context(), req.QueryParameter("envName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListComponentDefinitionResponse{ComponentDefinitions: componentDefinitions}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
