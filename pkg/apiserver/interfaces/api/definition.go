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

package api

import (
	"strconv"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	restful "github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

type definitionAPIInterface struct {
	DefinitionService service.DefinitionService `inject:""`
	RbacService       service.RBACService       `inject:""`
}

func (d *definitionAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/definitions").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for definition manage")

	tags := []string{"definition"}

	ws.Route(ws.GET("/").To(d.listDefinitions).
		Doc("list all definitions").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		// TODO: provide project scope api for query definition list
		// Filter(d.RbacService.CheckPerm("definition", "list")).
		Param(ws.QueryParameter("type", "query the definition type").DataType("string").Required(true).AllowableValues(map[string]string{"component": "", "trait": "", "workflowstep": ""})).
		Param(ws.QueryParameter("queryAll", "query all definitions include hidden in UI").DataType("boolean").DefaultValue("false")).
		Param(ws.QueryParameter("appliedWorkload", "if specified, query the trait definition applied to the workload").DataType("string")).
		Param(ws.QueryParameter("ownerAddon", "query by which addon created the definition").DataType("string")).
		Returns(200, "OK", apis.ListDefinitionResponse{}).
		Writes(apis.ListDefinitionResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{definitionName}").To(d.detailDefinition).
		Doc("Detail a definition").
		// Filter(d.RbacService.CheckPerm("definition", "detail")).
		Param(ws.PathParameter("definitionName", "identifier of the definition").DataType("string")).
		Param(ws.QueryParameter("type", "query the definition type").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "create successfully", apis.DetailDefinitionResponse{}).
		Writes(apis.DetailDefinitionResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{definitionName}/uischema").To(d.updateUISchema).
		Doc("Update the UI schema for a definition").
		Filter(d.RbacService.CheckPerm("definition", "update")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateUISchemaRequest{}).
		Returns(200, "update successfully", utils.UISchema{}).
		Writes(apis.DetailDefinitionResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{definitionName}/status").To(d.updateDefinitionStatus).
		Doc("Update the status for a definition").
		Filter(d.RbacService.CheckPerm("definition", "update")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateDefinitionStatusRequest{}).
		Returns(200, "update successfully", utils.UISchema{}).
		Writes(apis.DetailDefinitionResponse{}).Do(returns200, returns500))

	ws.Filter(authCheckFilter)
	return ws
}

// NewDefinitionAPIInterface new definition APIInterface
func NewDefinitionAPIInterface() Interface {
	return &definitionAPIInterface{}
}

func (d *definitionAPIInterface) listDefinitions(req *restful.Request, res *restful.Response) {
	queryAll, err := strconv.ParseBool(req.QueryParameter("queryAll"))
	if err != nil {
		queryAll = false
	}
	definitions, err := d.DefinitionService.ListDefinitions(req.Request.Context(), service.DefinitionQueryOption{
		Type:             req.QueryParameter("type"),
		AppliedWorkloads: req.QueryParameter("appliedWorkload"),
		OwnerAddon:       req.QueryParameter("ownerAddon"),
		QueryAll:         queryAll,
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListDefinitionResponse{Definitions: definitions}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (d *definitionAPIInterface) detailDefinition(req *restful.Request, res *restful.Response) {
	definition, err := d.DefinitionService.DetailDefinition(req.Request.Context(), req.PathParameter("definitionName"), req.QueryParameter("type"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(definition); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (d *definitionAPIInterface) updateUISchema(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateUISchemaRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := updateReq.UISchema.Validate(); err != nil {
		bcode.ReturnError(req, res, bcode.ErrInvalidDefinitionUISchema.SetMessage(err.Error()))
		return
	}
	schema, err := d.DefinitionService.AddDefinitionUISchema(req.Request.Context(), req.PathParameter("definitionName"), updateReq.DefinitionType, updateReq.UISchema)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(schema); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (d *definitionAPIInterface) updateDefinitionStatus(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateDefinitionStatusRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	schema, err := d.DefinitionService.UpdateDefinitionStatus(req.Request.Context(), req.PathParameter("definitionName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(schema); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
