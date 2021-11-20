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
	"strconv"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	restful "github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// NewWorkflowWebService new workflow webservice
func NewWorkflowWebService(workflowUsecase usecase.WorkflowUsecase, applicationUsecase usecase.ApplicationUsecase) WebService {
	return &workflowWebService{
		workflowUsecase:    workflowUsecase,
		applicationUsecase: applicationUsecase,
	}
}

type workflowWebService struct {
	workflowUsecase    usecase.WorkflowUsecase
	applicationUsecase usecase.ApplicationUsecase
}

func (w *workflowWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/workflows").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for cluster manage")

	tags := []string{"workflow"}

	ws.Route(ws.GET("/").To(w.listApplicationWorkflows).
		Doc("list application workflow").
		Param(ws.QueryParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.QueryParameter("enable", "query based on enable status").DataType("boolean")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ListWorkflowResponse{}).
		Writes(apis.ListWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.POST("/").To(w.createApplicationWorkflow).
		Doc("create application workflow").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateWorkflowRequest{}).
		Returns(200, "create success", apis.DetailWorkflowResponse{}).
		Returns(400, "create failure", bcode.Bcode{}).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}").To(w.detailWorkflow).
		Doc("detail application workflow").
		Param(ws.PathParameter("name", "identifier of the workflow.").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(w.workflowCheckFilter).
		Returns(200, "create success", apis.DetailWorkflowResponse{}).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{name}").To(w.updateWorkflow).
		Doc("update application workflow config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(w.workflowCheckFilter).
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Reads(apis.UpdateWorkflowRequest{}).
		Returns(200, "", apis.DetailWorkflowResponse{}).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.DELETE("/{name}").To(w.deleteWorkflow).
		Doc("deletet workflow").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(w.workflowCheckFilter).
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}/records").To(w.listWorkflowRecords).
		Doc("query application workflow execution record").
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(w.workflowCheckFilter).
		Param(ws.QueryParameter("page", "query the page number").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "query the page size number").DataType("integer")).
		Returns(200, "", apis.ListWorkflowRecordsResponse{}).
		Writes(apis.ListWorkflowRecordsResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}/records/{record}").To(w.detailWorkflowRecord).
		Doc("query application workflow execution record detail").
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the workflow record").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DetailWorkflowRecordResponse{}).
		Writes(apis.DetailWorkflowRecordResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}/records/{record}/resume").To(w.resumeWorkflowRecord).
		Doc("resume suspend workflow record").
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the  workflow record").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(w.applicationCheckFilter).
		Returns(200, "", nil).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailWorkflowRecordResponse{}))

	ws.Route(ws.GET("/{name}/records/{record}/terminate").To(w.terminateWorkflowRecord).
		Doc("terminate suspend workflow record").
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the workflow record").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(w.applicationCheckFilter).
		Returns(200, "", nil).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailWorkflowRecordResponse{}))

	ws.Route(ws.GET("/{name}/records/{record}/rollback").To(w.rollbackWorkflowRecord).
		Doc("rollback suspend application record").
		Param(ws.PathParameter("name", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the workflow record").DataType("string")).
		Param(ws.QueryParameter("rollbackVersion", "identifier of the rollback revision").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(w.applicationCheckFilter).
		Returns(200, "", nil).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DetailWorkflowRecordResponse{}))
	return ws
}

func (w *workflowWebService) workflowCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	workflow, err := w.workflowUsecase.GetWorkflow(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyWorkflow, workflow))
	chain.ProcessFilter(req, res)
}

func (w *workflowWebService) applicationCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	workflow, err := w.workflowUsecase.GetWorkflow(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	app, err := w.applicationUsecase.GetApplication(req.Request.Context(), workflow.AppPrimaryKey)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplication, app))
	chain.ProcessFilter(req, res)
}

func (w *workflowWebService) listApplicationWorkflows(req *restful.Request, res *restful.Response) {
	if req.QueryParameter("appName") == "" {
		bcode.ReturnError(req, res, bcode.ErrMustQueryByApp)
		return
	}
	app, err := w.applicationUsecase.GetApplication(req.Request.Context(), req.QueryParameter("appName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	var enableQuery *bool
	enable, err := strconv.ParseBool(req.QueryParameter("enable"))
	if err == nil {
		enableQuery = &enable
	}
	workflows, err := w.workflowUsecase.ListApplicationWorkflow(req.Request.Context(), app, enableQuery)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListWorkflowResponse{Workflows: workflows}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *workflowWebService) createApplicationWorkflow(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateWorkflowRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	app, err := w.applicationUsecase.GetApplication(req.Request.Context(), createReq.AppName)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the usecase layer code
	workflowDetail, err := w.workflowUsecase.CreateWorkflow(req.Request.Context(), app, createReq)
	if err != nil {
		log.Logger.Errorf("create application failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(workflowDetail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *workflowWebService) detailWorkflow(req *restful.Request, res *restful.Response) {
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	detail, err := w.workflowUsecase.DetailWorkflow(req.Request.Context(), workflow)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *workflowWebService) updateWorkflow(req *restful.Request, res *restful.Response) {
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	// Verify the validity of parameters
	var updateReq apis.UpdateWorkflowRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	detail, err := w.workflowUsecase.UpdateWorkflow(req.Request.Context(), workflow, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *workflowWebService) deleteWorkflow(req *restful.Request, res *restful.Response) {
	if err := w.workflowUsecase.DeleteWorkflow(req.Request.Context(), req.PathParameter("name")); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *workflowWebService) listWorkflowRecords(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	records, err := w.workflowUsecase.ListWorkflowRecords(req.Request.Context(), req.PathParameter("name"), page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(records); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *workflowWebService) detailWorkflowRecord(req *restful.Request, res *restful.Response) {
	record, err := w.workflowUsecase.DetailWorkflowRecord(req.Request.Context(), req.PathParameter("name"), req.PathParameter("record"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(record); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *workflowWebService) resumeWorkflowRecord(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := w.workflowUsecase.ResumeRecord(req.Request.Context(), app, req.PathParameter("record"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	return
}

func (w *workflowWebService) terminateWorkflowRecord(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := w.workflowUsecase.TerminateRecord(req.Request.Context(), app, req.PathParameter("record"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	return
}

func (w *workflowWebService) rollbackWorkflowRecord(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := w.workflowUsecase.RollbackRecord(req.Request.Context(), app, req.PathParameter("record"), req.QueryParameter("rollbackVersion"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	return
}
