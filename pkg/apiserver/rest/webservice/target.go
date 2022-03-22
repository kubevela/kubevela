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
	"context"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// NewTargetWebService new Target webservice
func NewTargetWebService(targetUsecase usecase.TargetUsecase, applicationUsecase usecase.ApplicationUsecase) WebService {
	return &TargetWebService{
		TargetUsecase:      targetUsecase,
		applicationUsecase: applicationUsecase,
	}
}

// TargetWebService  target web service
type TargetWebService struct {
	TargetUsecase      usecase.TargetUsecase
	applicationUsecase usecase.ApplicationUsecase
}

// GetWebService get web service
func (dt *TargetWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/targets").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for Target manage")

	tags := []string{"Target"}

	ws.Route(ws.GET("/").To(dt.listTargets).
		Doc("list Target").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("page", "Page for paging").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "PageSize for paging").DataType("integer")).
		Returns(200, "OK", apis.ListTargetResponse{}).
		Writes(apis.ListTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.POST("/").To(dt.createTarget).
		Doc("create Target").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateTargetRequest{}).
		Returns(200, "create success", apis.DetailTargetResponse{}).
		Returns(400, "create failure", bcode.Bcode{}).
		Writes(apis.DetailTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}").To(dt.detailTarget).
		Doc("detail Target").
		Param(ws.PathParameter("name", "identifier of the Target.").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.targetCheckFilter).
		Returns(200, "create success", apis.DetailTargetResponse{}).
		Writes(apis.DetailTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{name}").To(dt.updateTarget).
		Doc("update application Target config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.targetCheckFilter).
		Param(ws.PathParameter("name", "identifier of the Target").DataType("string")).
		Reads(apis.UpdateTargetRequest{}).
		Returns(200, "OK", apis.DetailTargetResponse{}).
		Writes(apis.DetailTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.DELETE("/{name}").To(dt.deleteTarget).
		Doc("deletet Target").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.targetCheckFilter).
		Param(ws.PathParameter("name", "identifier of the Target").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	ws.Filter(authCheckFilter)
	return ws
}

func (dt *TargetWebService) createTarget(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	TargetDetail, err := dt.TargetUsecase.CreateTarget(req.Request.Context(), createReq)
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

func (dt *TargetWebService) targetCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	Target, err := dt.TargetUsecase.GetTarget(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyTarget, Target))
	chain.ProcessFilter(req, res)
}

func (dt *TargetWebService) detailTarget(req *restful.Request, res *restful.Response) {
	Target := req.Request.Context().Value(&apis.CtxKeyTarget).(*model.Target)
	detail, err := dt.TargetUsecase.DetailTarget(req.Request.Context(), Target)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *TargetWebService) updateTarget(req *restful.Request, res *restful.Response) {
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
	detail, err := dt.TargetUsecase.UpdateTarget(req.Request.Context(), Target, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *TargetWebService) deleteTarget(req *restful.Request, res *restful.Response) {
	TargetName := req.PathParameter("name")
	// Target in use, can't be deleted
	applications, err := dt.applicationUsecase.ListApplications(req.Request.Context(), apis.ListApplicationOptions{TargetName: TargetName})
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
	if err := dt.TargetUsecase.DeleteTarget(req.Request.Context(), TargetName); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *TargetWebService) listTargets(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	Targets, err := dt.TargetUsecase.ListTargets(req.Request.Context(), page, pageSize, req.QueryParameter("project"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(Targets); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
