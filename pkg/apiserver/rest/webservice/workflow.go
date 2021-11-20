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

	restful "github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type workflowWebService struct {
	workflowUsecase    usecase.WorkflowUsecase
	applicationUsecase usecase.ApplicationUsecase
}

func (w *workflowWebService) workflowCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflow, err := w.workflowUsecase.GetWorkflow(req.Request.Context(), app, req.PathParameter("workflowName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyWorkflow, workflow))
	chain.ProcessFilter(req, res)
}

func (w *workflowWebService) applicationCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflow, err := w.workflowUsecase.GetWorkflow(req.Request.Context(), app, req.PathParameter("workflowName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyWorkflow, workflow))
	chain.ProcessFilter(req, res)
}

func (w *workflowWebService) listApplicationWorkflows(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflows, err := w.workflowUsecase.ListApplicationWorkflow(req.Request.Context(), app)
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	if err := w.workflowUsecase.DeleteWorkflow(req.Request.Context(), app, req.PathParameter("workflowName")); err != nil {
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
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	records, err := w.workflowUsecase.ListWorkflowRecords(req.Request.Context(), workflow, page, pageSize)
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
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	record, err := w.workflowUsecase.DetailWorkflowRecord(req.Request.Context(), workflow, req.PathParameter("record"))
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
