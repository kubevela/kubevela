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
	"context"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

// NewTargetAPIInterface new Target Interface
func NewTargetAPIInterface() Interface {
	return &TargetAPIInterface{}
}

// TargetAPIInterface  target web service
type TargetAPIInterface struct {
	TargetService      service.TargetService      `inject:""`
	ApplicationService service.ApplicationService `inject:""`
	RbacService        service.RBACService        `inject:""`
}

// GetWebServiceRoute get web service
func (dt *TargetAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/targets").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for Target manage")

	tags := []string{"Target"}

	ws.Route(ws.GET("/").To(dt.listTargets).
		Doc("list Target").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.RbacService.CheckPerm("target", "list")).
		Param(ws.QueryParameter("page", "Page for paging").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "PageSize for paging").DataType("integer")).
		Param(ws.QueryParameter("project", "list targets by project name").DataType("string")).
		Returns(200, "OK", apis.ListTargetResponse{}).
		Writes(apis.ListTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.POST("/").To(dt.createTarget).
		Doc("create Target").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateTargetRequest{}).
		Filter(dt.RbacService.CheckPerm("target", "create")).
		Returns(200, "create success", apis.DetailTargetResponse{}).
		Returns(400, "create failure", bcode.Bcode{}).
		Writes(apis.DetailTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{targetName}").To(dt.detailTarget).
		Doc("detail Target").
		Param(ws.PathParameter("targetName", "identifier of the Target.").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.targetCheckFilter).
		Filter(dt.RbacService.CheckPerm("target", "detail")).
		Returns(200, "create success", apis.DetailTargetResponse{}).
		Writes(apis.DetailTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{targetName}").To(dt.updateTarget).
		Doc("update application Target config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.targetCheckFilter).
		Param(ws.PathParameter("targetName", "identifier of the Target").DataType("string")).
		Reads(apis.UpdateTargetRequest{}).
		Filter(dt.RbacService.CheckPerm("target", "update")).
		Returns(200, "OK", apis.DetailTargetResponse{}).
		Writes(apis.DetailTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.DELETE("/{targetName}").To(dt.deleteTarget).
		Doc("deletet Target").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.targetCheckFilter).
		Filter(dt.RbacService.CheckPerm("target", "delete")).
		Param(ws.PathParameter("targetName", "identifier of the Target").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	ws.Filter(authCheckFilter)
	return ws
}

func (dt *TargetAPIInterface) createTarget(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateTargetRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	TargetDetail, err := dt.TargetService.CreateTarget(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create -target failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(TargetDetail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *TargetAPIInterface) targetCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	Target, err := dt.TargetService.GetTarget(req.Request.Context(), req.PathParameter("targetName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyTarget, Target))
	chain.ProcessFilter(req, res)
}

func (dt *TargetAPIInterface) detailTarget(req *restful.Request, res *restful.Response) {
	Target := req.Request.Context().Value(&apis.CtxKeyTarget).(*model.Target)
	detail, err := dt.TargetService.DetailTarget(req.Request.Context(), Target)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *TargetAPIInterface) updateTarget(req *restful.Request, res *restful.Response) {
	Target := req.Request.Context().Value(&apis.CtxKeyTarget).(*model.Target)
	// Verify the validity of parameters
	var updateReq apis.UpdateTargetRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	detail, err := dt.TargetService.UpdateTarget(req.Request.Context(), Target, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *TargetAPIInterface) deleteTarget(req *restful.Request, res *restful.Response) {
	TargetName := req.PathParameter("targetName")
	// Target in use, can't be deleted
	applications, err := dt.ApplicationService.ListApplications(req.Request.Context(), apis.ListApplicationOptions{TargetName: TargetName})
	if err != nil {
		if !errors.Is(err, datastore.ErrRecordNotExist) {
			bcode.ReturnError(req, res, err)
			return
		}
	}
	if applications != nil {
		bcode.ReturnError(req, res, bcode.ErrTargetInUseCantDeleted)
		return
	}
	if err := dt.TargetService.DeleteTarget(req.Request.Context(), TargetName); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *TargetAPIInterface) listTargets(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	Targets, err := dt.TargetService.ListTargets(req.Request.Context(), page, pageSize, req.QueryParameter("project"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(Targets); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
