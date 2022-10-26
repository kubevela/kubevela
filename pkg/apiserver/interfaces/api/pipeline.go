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

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

type pipelineAPIInterface struct {
	PipelineService    service.PipelineService    `inject:""`
	PipelineRunService service.PipelineRunService `inject:""`
	ContextService     service.ContextService     `inject:""`
}

type pipelinePathParamKey string

const (
	// Project is the project name key of query param
	Project pipelinePathParamKey = "projectName"
	// Pipeline is the pipeline name of query param
	Pipeline pipelinePathParamKey = "pipelineName"
	// PipelineRun is the pipeline run name of query param
	PipelineRun pipelinePathParamKey = "runName"
	// ContextName is the context name of query param
	ContextName pipelinePathParamKey = "contextName"
)

// GetWebServiceRoute is the implementation of pipeline Interface
func (p *pipelineAPIInterface) GetWebServiceRoute() *restful.WebService {

	ws := new(restful.WebService)
	tags := []string{"pipeline"}

	projParam := func(builder *restful.RouteBuilder) {
		builder.Param(ws.PathParameter(string(Project), "project name").Required(true))
	}
	pipelineParam := func(builder *restful.RouteBuilder) {
		builder.Param(ws.PathParameter(string(Pipeline), "pipeline name").Required(true))
		builder.Filter(p.pipelineCheckFilter)
	}
	ctxParam := func(builder *restful.RouteBuilder) {
		builder.Param(ws.PathParameter(string(ContextName), "pipeline context name").Required(true))
		builder.Filter(p.pipelineContextCheckFilter)
	}
	runParam := func(builder *restful.RouteBuilder) {
		builder.Param(ws.PathParameter(string(PipelineRun), "pipeline run name").Required(true))
		builder.Filter(p.pipelineRunCheckFilter)
	}
	meta := func(builder *restful.RouteBuilder) {
		builder.Metadata(restfulspec.KeyOpenAPITags, tags)
	}

	ws.Path(versionPrefix).
		Consumes(restful.MIME_JSON, restful.MIME_XML).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for pipeline manage")

	ws.Route(ws.POST("/").To(p.createPipeline).
		Doc("create pipeline").
		Reads(apis.CreatePipelineRequest{}).
		Returns(200, "OK", apis.PipelineBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PipelineBase{}).Do(meta))

	ws.Route(ws.GET("").To(p.listPipelines).
		Doc("list pipelines").
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Returns(200, "OK", apis.ListPipelineResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListPipelineResponse{}).Do(meta))

	ws.Route(ws.GET("/project/{projectName}/pipelines/{pipelineName}").To(p.getPipeline).
		Doc("get pipeline").
		Reads(apis.GetPipelineRequest{}).
		Returns(200, "OK", apis.GetPipelineResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.GetPipelineResponse{}).Do(meta, projParam, pipelineParam))

	ws.Route(ws.PUT("/project/{projectName}/pipelines/{pipelineName}").To(p.updatePipeline).
		Doc("update pipeline").
		Reads(apis.UpdatePipelineRequest{}).
		Returns(200, "OK", apis.PipelineBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PipelineBase{}).Do(meta, projParam, pipelineParam))

	ws.Route(ws.DELETE("/project/{projectName}/pipelines/{pipelineName}").To(p.deletePipeline).
		Doc("delete pipeline").
		Returns(200, "OK", apis.PipelineMetaResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PipelineMetaResponse{}).Do(meta, projParam, pipelineParam))

	ws.Route(ws.POST("/project/{projectName}/pipelines/{pipelineName}/contexts").To(p.createContextValue).
		Doc("create pipeline context values").
		Reads(apis.CreateContextValuesRequest{}).
		Returns(200, "OK", apis.Context{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.Context{}).Do(meta, projParam, pipelineParam))

	ws.Route(ws.GET("/project/{projectName}/pipelines/{pipelineName}/contexts").To(p.listContextValues).
		Doc("list pipeline context values").
		Returns(200, "OK", apis.ListContextValueResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListContextValueResponse{}).Do(meta, projParam, pipelineParam))

	ws.Route(ws.PUT("/project/{projectName}/pipelines/{pipelineName}/contexts/{contextName}").To(p.updateContextValue).
		Doc("update pipeline context value").
		Reads(apis.UpdateContextValuesRequest{}).
		Returns(200, "OK", apis.Context{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.Context{}).Do(meta, projParam, pipelineParam, ctxParam))

	ws.Route(ws.DELETE("/project/{projectName}/pipelines/{pipelineName}/contexts/{contextName}").To(p.deleteContextValue).
		Doc("delete pipeline context value").
		Returns(200, "OK", apis.ContextNameResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ContextNameResponse{}).Do(meta, projParam, pipelineParam, ctxParam))

	ws.Route(ws.POST("/project/{projectName}/pipelines/{pipelineName}/run").To(p.runPipeline).
		Doc("run pipeline").
		Reads(apis.RunPipelineRequest{}).
		Returns(200, "OK", apis.PipelineRunMeta{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PipelineRunMeta{}).Do(meta, projParam, pipelineParam))

	ws.Route(ws.GET("/project/{projectName}/pipelines/{pipelineName}/runs").To(p.listPipelineRuns).
		Doc("list pipeline runs").
		Param(ws.QueryParameter("status", "query identifier of the status").DataType("string")).
		Returns(200, "OK", apis.ListPipelineRunResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListPipelineRunResponse{}).Do(meta, projParam, pipelineParam))

	ws.Route(ws.POST("/project/{projectName}/pipelines/{pipelineName}/runs/{runName}/stop").To(p.stopPipeline).
		Doc("stop pipeline run").
		Returns(200, "OK", apis.PipelineRunMeta{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PipelineRunMeta{}).Do(meta, projParam, pipelineParam, runParam))

	ws.Route(ws.GET("/project/{projectName}/pipelines/{pipelineName}/runs/{runName}").To(p.getPipelineRun).
		Doc("get pipeline run").
		Returns(200, "OK", apis.PipelineRunBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PipelineRunBase{}).Do(meta, projParam, pipelineParam, runParam))

	ws.Route(ws.DELETE("/project/{projectName}/pipelines/{pipelineName}/runs/{runName}").To(p.deletePipelineRun).
		Doc("delete pipeline run").
		Returns(200, "OK", apis.PipelineRunMeta{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PipelineRunMeta{}).Do(meta, projParam, pipelineParam, runParam))

	// get pipeline run status
	ws.Route(ws.GET("/project/{projectName}/pipelines/{pipelineName}/runs/{runName}/status").To(p.getPipelineRunStatus).
		Doc("get pipeline run status").
		Returns(200, "OK", workflowv1alpha1.WorkflowRunStatus{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(workflowv1alpha1.WorkflowRunStatus{}).Do(meta, projParam, pipelineParam, runParam))

	// get pipeline run log
	ws.Route(ws.GET("/project/{projectName}/pipelines/{pipelineName}/runs/{runName}/log").To(p.getPipelineRunLog).
		Doc("get pipeline run log").
		Param(ws.QueryParameter("step", "query by specific step name").DataType("string")).
		Returns(200, "OK", apis.GetPipelineRunLogResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.GetPipelineRunLogResponse{}).Do(meta, projParam, pipelineParam, runParam))

	// get pipeline run output
	ws.Route(ws.GET("/project/{projectName}/pipelines/{pipelineName}/runs/{runName}/output").To(p.getPipelineRunOutput).
		Doc("get pipeline run output").
		Param(ws.QueryParameter("step", "query by specific id").DataType("string")).
		Returns(200, "OK", apis.GetPipelineRunOutputResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.GetPipelineRunOutputResponse{}).Do(meta, projParam, pipelineParam, runParam))

	ws.Filter(authCheckFilter)
	return ws
}

// NewPipelineAPIInterface new pipeline manage APIInterface
func NewPipelineAPIInterface() Interface {
	return &pipelineAPIInterface{}
}

func (p *pipelineAPIInterface) listPipelines(req *restful.Request, res *restful.Response) {
	var projetNames []string
	if req.QueryParameter("project") != "" {
		projetNames = append(projetNames, req.QueryParameter("project"))
	}
	pipelines, err := p.PipelineService.ListPipelines(req.Request.Context(), apis.ListPipelineRequest{
		Projects: projetNames,
		Query:    req.QueryParameter("query"),
	})
	if err != nil {
		log.Logger.Errorf("list pipeline failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipelines); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) getPipeline(req *restful.Request, res *restful.Response) {
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	if err := res.WriteEntity(pipeline); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) createPipeline(req *restful.Request, res *restful.Response) {
	var createReq apis.CreatePipelineRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	pipelineBase, err := p.PipelineService.CreatePipeline(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create pipeline failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	err = p.ContextService.InitContext(req.Request.Context(), pipelineBase.Project, pipelineBase.Name)
	if err != nil {
		log.Logger.Errorf("init pipeline context failure: %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(pipelineBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) updatePipeline(req *restful.Request, res *restful.Response) {
	var updateReq apis.UpdatePipelineRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	pipelineBase, err := p.PipelineService.UpdatePipeline(req.Request.Context(), base.Name, base.Project, updateReq)
	if err != nil {
		log.Logger.Errorf("update pipeline failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipelineBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) deletePipeline(req *restful.Request, res *restful.Response) {
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	err := p.PipelineService.DeletePipeline(req.Request.Context(), pipeline)
	if err != nil {
		log.Logger.Errorf("delete pipeline failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := p.ContextService.DeleteAllContexts(req.Request.Context(), pipeline.Project, pipeline.Name); err != nil {
		log.Logger.Errorf("delete pipeline all context failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipeline.PipelineMeta); err != nil {
		log.Logger.Errorf("delete pipeline failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) runPipeline(req *restful.Request, res *restful.Response) {
	var runReq apis.RunPipelineRequest
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	if err := req.ReadEntity(&runReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	err := p.PipelineService.RunPipeline(req.Request.Context(), pipeline, runReq)
	if err != nil {
		log.Logger.Errorf("run pipeline failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipeline.PipelineMeta); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) stopPipeline(req *restful.Request, res *restful.Response) {
	pipelineRun := req.Request.Context().Value(apis.CtxKeyPipelineRun).(apis.PipelineRun)
	err := p.PipelineRunService.StopPipelineRun(req.Request.Context(), pipelineRun.PipelineRunBase)
	if err != nil {
		log.Logger.Errorf("stop pipeline failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipelineRun.PipelineRunMeta); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) listPipelineRuns(req *restful.Request, res *restful.Response) {
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	pipelineRuns, err := p.PipelineRunService.ListPipelineRuns(req.Request.Context(), pipeline)
	if err != nil {
		log.Logger.Errorf("list pipeline runs failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipelineRuns); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) getPipelineRun(req *restful.Request, res *restful.Response) {
	pipelineRun := req.Request.Context().Value(apis.CtxKeyPipelineRun).(apis.PipelineRun)
	if err := res.WriteEntity(pipelineRun.PipelineRunBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) getPipelineRunStatus(req *restful.Request, res *restful.Response) {
	pipelineRun := req.Request.Context().Value(apis.CtxKeyPipelineRun).(apis.PipelineRun)
	if err := res.WriteEntity(pipelineRun.Status); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) getPipelineRunLog(req *restful.Request, res *restful.Response) {
	pipelineRun := req.Request.Context().Value(apis.CtxKeyPipelineRun).(apis.PipelineRun)
	step := req.QueryParameter("step")
	logs, err := p.PipelineRunService.GetPipelineRunLog(req.Request.Context(), pipelineRun, step)
	if err != nil {
		log.Logger.Errorf("get pipeline run log failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(logs); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) getPipelineRunOutput(req *restful.Request, res *restful.Response) {
	pipelineRun := req.Request.Context().Value(apis.CtxKeyPipelineRun).(apis.PipelineRun)
	output, err := p.PipelineRunService.GetPipelineRunOutput(req.Request.Context(), pipelineRun)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(output); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) deletePipelineRun(req *restful.Request, res *restful.Response) {
	pipelineRun := req.Request.Context().Value(apis.CtxKeyPipelineRun).(apis.PipelineRun)
	err := p.PipelineRunService.DeletePipelineRun(req.Request.Context(), pipelineRun.PipelineRunMeta)
	if err != nil {
		log.Logger.Errorf("delete pipeline run failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipelineRun.PipelineRunMeta); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) listContextValues(req *restful.Request, res *restful.Response) {
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	contextValues, err := p.ContextService.ListContexts(req.Request.Context(), pipeline.Project, pipeline.Name)
	if err != nil {
		log.Logger.Errorf("list context values failure: %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(contextValues); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) createContextValue(req *restful.Request, res *restful.Response) {
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	var createReq apis.CreateContextValuesRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	pipelineCtx := apis.Context(createReq)
	_, err := p.ContextService.CreateContext(req.Request.Context(), pipeline.Project, pipeline.Name, pipelineCtx)
	if err != nil {
		log.Logger.Errorf("create context failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipelineCtx); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) updateContextValue(req *restful.Request, res *restful.Response) {
	plCtx := req.Request.Context().Value(apis.CtxKeyPipelineContex).(apis.Context)
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	var updateReq apis.UpdateContextValuesRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	pipelineCtx := apis.Context{Name: plCtx.Name, Values: updateReq.Values}
	_, err := p.ContextService.UpdateContext(req.Request.Context(), pipeline.Project, pipeline.Name, pipelineCtx)
	if err != nil {
		log.Logger.Errorf("update context failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(pipelineCtx); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) deleteContextValue(req *restful.Request, res *restful.Response) {
	plCtx := req.Request.Context().Value(apis.CtxKeyPipelineContex).(apis.Context)
	pipeline := req.Request.Context().Value(apis.CtxKeyPipeline).(apis.PipelineBase)
	err := p.ContextService.DeleteContext(req.Request.Context(), pipeline.Project, pipeline.Name, plCtx.Name)
	if err != nil {
		log.Logger.Errorf("delete context failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ContextNameResponse{Name: plCtx.Name}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (p *pipelineAPIInterface) pipelineCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	pipeline, err := p.PipelineService.GetPipeline(req.Request.Context(), req.PathParameter("pipelineName"), req.QueryParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), apis.CtxKeyPipeline, pipeline.PipelineBase))
	chain.ProcessFilter(req, res)
}

func (p *pipelineAPIInterface) pipelineContextCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	contexts, err := p.ContextService.ListContexts(req.Request.Context(), req.PathParameter("pipelineName"), req.QueryParameter("projectName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	contextName := req.PathParameter("contextName")
	contextValue, ok := contexts.Contexts[contextName]
	if !ok {
		bcode.ReturnError(req, res, bcode.ErrContextNotFound)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), apis.CtxKeyPipelineContex, apis.Context{
		Name:   contextName,
		Values: contextValue,
	}))
	chain.ProcessFilter(req, res)
}

func (p *pipelineAPIInterface) pipelineRunCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	meta := apis.PipelineRunMeta{
		PipelineName:    req.PathParameter(string(Pipeline)),
		Project:         req.QueryParameter(string(Project)),
		PipelineRunName: req.PathParameter(string(PipelineRun)),
	}
	run, err := p.PipelineRunService.GetPipelineRun(req.Request.Context(), meta)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), apis.CtxKeyPipelineRun, run))

	chain.ProcessFilter(req, res)
}
