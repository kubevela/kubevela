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

	restful "github.com/emicklei/go-restful/v3"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

// Workflow workflow api
type Workflow struct {
	WorkflowService    service.WorkflowService    `inject:""`
	ApplicationService service.ApplicationService `inject:""`
}

// NewWorkflow new workflow api interface
func NewWorkflow() interface{} {
	return &Workflow{}
}

func (w *Workflow) workflowCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflow, err := w.WorkflowService.GetWorkflow(req.Request.Context(), app, req.PathParameter("workflowName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyWorkflow, workflow))
	chain.ProcessFilter(req, res)
}

func (w *Workflow) workflowRecordCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	record, err := w.WorkflowService.GetWorkflowRecord(req.Request.Context(), workflow, req.PathParameter("record"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyWorkflowRecord, record))
	chain.ProcessFilter(req, res)
}

func (w *Workflow) listApplicationWorkflows(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflows, err := w.WorkflowService.ListApplicationWorkflow(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListWorkflowResponse{Workflows: workflows}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) createOrUpdateApplicationWorkflow(req *restful.Request, res *restful.Response) {
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
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Call the domain service layer code
	workflowDetail, err := w.WorkflowService.CreateOrUpdateWorkflow(req.Request.Context(), app, createReq)
	if err != nil {
		klog.Errorf("create application failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(workflowDetail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) detailWorkflow(req *restful.Request, res *restful.Response) {
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	detail, err := w.WorkflowService.DetailWorkflow(req.Request.Context(), workflow)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) updateWorkflow(req *restful.Request, res *restful.Response) {
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
	detail, err := w.WorkflowService.UpdateWorkflow(req.Request.Context(), workflow, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) deleteWorkflow(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	if err := w.WorkflowService.DeleteWorkflow(req.Request.Context(), app, req.PathParameter("workflowName")); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) listWorkflowRecords(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	records, err := w.WorkflowService.ListWorkflowRecords(req.Request.Context(), workflow, page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(records); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) detailWorkflowRecord(req *restful.Request, res *restful.Response) {
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	record, err := w.WorkflowService.DetailWorkflowRecord(req.Request.Context(), workflow, req.PathParameter("record"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(record); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) resumeWorkflowRecord(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	err := w.WorkflowService.ResumeRecord(req.Request.Context(), app, workflow, req.PathParameter("record"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) terminateWorkflowRecord(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	err := w.WorkflowService.TerminateRecord(req.Request.Context(), app, workflow, req.PathParameter("record"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) rollbackWorkflowRecord(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	record, err := w.WorkflowService.RollbackRecord(req.Request.Context(), app, workflow, req.PathParameter("record"), req.QueryParameter("rollbackVersion"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(record); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) getWorkflowRecordLogs(req *restful.Request, res *restful.Response) {
	record := req.Request.Context().Value(&apis.CtxKeyWorkflowRecord).(*model.WorkflowRecord)
	logResponse, err := w.WorkflowService.GetWorkflowRecordLog(req.Request.Context(), record, req.QueryParameter("step"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(logResponse); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) getWorkflowRecordInputs(req *restful.Request, res *restful.Response) {
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	record := req.Request.Context().Value(&apis.CtxKeyWorkflowRecord).(*model.WorkflowRecord)
	inputs, err := w.WorkflowService.GetWorkflowRecordInput(req.Request.Context(), workflow, record, req.QueryParameter("step"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(inputs); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (w *Workflow) getWorkflowRecordOutputs(req *restful.Request, res *restful.Response) {
	workflow := req.Request.Context().Value(&apis.CtxKeyWorkflow).(*model.Workflow)
	record := req.Request.Context().Value(&apis.CtxKeyWorkflowRecord).(*model.WorkflowRecord)
	outputRes, err := w.WorkflowService.GetWorkflowRecordOutput(req.Request.Context(), workflow, record, req.QueryParameter("step"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(outputRes); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
