/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://wwc.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/apiserver/utils"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

type applicationAPIInterface struct {
	WorkflowAPI        WorkflowAPIInterface       `inject:"inline"`
	RbacService        service.RBACService        `inject:""`
	ApplicationService service.ApplicationService `inject:""`
	EnvBindingService  service.EnvBindingService  `inject:""`
}

// NewApplicationAPIInterface new application manage APIInterface
func NewApplicationAPIInterface() Interface {
	return &applicationAPIInterface{}
}

func (c *applicationAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/applications").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for application manage")

	tags := []string{"application"}

	ws.Route(ws.GET("/").To(c.listApplications).
		Doc("list all applications").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("query", "Fuzzy search based on name or description").DataType("string")).
		Param(ws.QueryParameter("project", "search base on project name").DataType("string")).
		Param(ws.QueryParameter("env", "search base on env name").DataType("string")).
		Param(ws.QueryParameter("targetName", "Name of the application delivery target").DataType("string")).
		// This api will filter the app by user's permissions
		// Filter(c.RbacService.CheckPerm("application", "list")).
		Returns(200, "OK", apis.ListApplicationResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListApplicationResponse{}))

	ws.Route(ws.POST("/").To(c.createApplication).
		Doc("create one application ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateApplicationRequest{}).
		Filter(c.RbacService.CheckPerm("application", "create")).
		Returns(200, "OK", apis.ApplicationBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationBase{}))

	ws.Route(ws.DELETE("/{appName}").To(c.deleteApplication).
		Doc("delete one application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "delete")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{appName}").To(c.detailApplication).
		Doc("detail one application ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "detail")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Returns(200, "OK", apis.DetailApplicationResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailApplicationResponse{}))

	ws.Route(ws.PUT("/{appName}").To(c.updateApplication).
		Doc("update one application ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "update")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Reads(apis.UpdateApplicationRequest{}).
		Returns(200, "OK", apis.ApplicationBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationBase{}))

	ws.Route(ws.GET("/{appName}/statistics").To(c.applicationStatistics).
		Doc("detail one application ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "detail")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Returns(200, "OK", apis.ApplicationStatisticsResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationStatisticsResponse{}))

	ws.Route(ws.POST("/{appName}/triggers").To(c.createApplicationTrigger).
		Doc("create one application trigger").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("trigger", "create")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Reads(apis.CreateApplicationTriggerRequest{}).
		Returns(200, "OK", apis.ApplicationTriggerBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationTriggerBase{}))

	ws.Route(ws.DELETE("/{appName}/triggers/{token}").To(c.deleteApplicationTrigger).
		Doc("delete one application trigger").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("trigger", "delete")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("token", "identifier of the trigger").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]*apis.EmptyResponse{}))

	ws.Route(ws.GET("/{appName}/triggers").To(c.listApplicationTriggers).
		Doc("list application triggers").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("trigger", "list")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Returns(200, "OK", apis.ListApplicationTriggerResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]*apis.ApplicationTriggerBase{}))

	ws.Route(ws.POST("/{appName}/template").To(c.publishApplicationTemplate).
		Doc("create one application template").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("applicationTemplate", "create")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Reads(apis.CreateApplicationTemplateRequest{}).
		Returns(200, "OK", apis.ApplicationTemplateBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationTemplateBase{}))

	ws.Route(ws.POST("/{appName}/deploy").To(c.deployApplication).
		Doc("deploy or upgrade the application").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "deploy")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Reads(apis.ApplicationDeployRequest{}).
		Returns(200, "OK", apis.ApplicationDeployResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationDeployResponse{}))

	ws.Route(ws.GET("/{appName}/components").To(c.listApplicationComponents).
		Doc("gets the list of application components").
		Filter(c.RbacService.CheckPerm("component", "list")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Param(ws.QueryParameter("envName", "list components that deployed in define env").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.ComponentListResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ComponentListResponse{}))

	ws.Route(ws.POST("/{appName}/components").To(c.createComponent).
		Doc("create component  for application ").
		Filter(c.RbacService.CheckPerm("component", "create")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateComponentRequest{}).
		Returns(200, "OK", apis.ComponentBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ComponentBase{}))

	ws.Route(ws.GET("/{appName}/components/{compName}").To(c.detailComponent).
		Doc("detail component for application ").
		Filter(c.RbacService.CheckPerm("component", "detail")).
		Filter(c.appCheckFilter).
		Filter(c.componentCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.DetailComponentResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailComponentResponse{}))

	ws.Route(ws.PUT("/{appName}/components/{compName}").To(c.updateComponent).
		Doc("update component config").
		Filter(c.RbacService.CheckPerm("component", "update")).
		Filter(c.appCheckFilter).
		Filter(c.componentCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateApplicationComponentRequest{}).
		Returns(200, "OK", apis.ComponentBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ComponentBase{}))

	ws.Route(ws.DELETE("/{appName}/components/{compName}").To(c.deleteComponent).
		Doc("delete a component").
		Filter(c.RbacService.CheckPerm("component", "delete")).
		Filter(c.appCheckFilter).
		Filter(c.componentCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{appName}/policies").To(c.listApplicationPolicies).
		Doc("list policy for application").
		Filter(c.RbacService.CheckPerm("policy", "list")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.ListApplicationPolicy{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListApplicationPolicy{}))

	ws.Route(ws.POST("/{appName}/policies").To(c.createApplicationPolicy).
		Doc("create policy for application").
		Filter(c.RbacService.CheckPerm("policy", "create")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreatePolicyRequest{}).
		Returns(200, "OK", apis.PolicyBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.PolicyBase{}))

	ws.Route(ws.GET("/{appName}/policies/{policyName}").To(c.detailApplicationPolicy).
		Doc("detail policy for application").
		Filter(c.RbacService.CheckPerm("policy", "detail")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.DetailPolicyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailPolicyResponse{}))

	ws.Route(ws.DELETE("/{appName}/policies/{policyName}").To(c.deleteApplicationPolicy).
		Doc("detail policy for application").
		Filter(c.RbacService.CheckPerm("policy", "delete")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.PUT("/{appName}/policies/{policyName}").To(c.updateApplicationPolicy).
		Doc("update policy for application").
		Filter(c.RbacService.CheckPerm("policy", "update")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("policyName", "identifier of the application policy").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdatePolicyRequest{}).
		Returns(200, "OK", apis.DetailPolicyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailPolicyResponse{}))

	ws.Route(ws.POST("/{appName}/components/{compName}/traits").To(c.addApplicationTrait).
		Doc("add trait for a component").
		Filter(c.RbacService.CheckPerm("trait", "create")).
		Filter(c.appCheckFilter).
		Filter(c.componentCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateApplicationTraitRequest{}).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationTrait{}))

	ws.Route(ws.PUT("/{appName}/components/{compName}/traits/{traitType}").To(c.updateApplicationTrait).
		Doc("update trait from a component").
		Filter(c.RbacService.CheckPerm("trait", "update")).
		Filter(c.appCheckFilter).
		Filter(c.componentCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Param(ws.PathParameter("traitType", "identifier of the type of trait").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.UpdateApplicationTraitRequest{}).
		Returns(200, "OK", apis.ApplicationTrait{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationTrait{}))

	ws.Route(ws.DELETE("/{appName}/components/{compName}/traits/{traitType}").To(c.deleteApplicationTrait).
		Doc("delete trait from a component").
		Filter(c.RbacService.CheckPerm("trait", "delete")).
		Filter(c.appCheckFilter).
		Filter(c.componentCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("compName", "identifier of the component").DataType("string")).
		Param(ws.PathParameter("traitType", "identifier of the type of trait").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.ApplicationTrait{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{appName}/revisions").To(c.listApplicationRevisions).
		Doc("list revisions for application").
		Filter(c.RbacService.CheckPerm("revision", "list")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Param(ws.QueryParameter("envName", "query identifier of the env").DataType("string")).
		Param(ws.QueryParameter("status", "query identifier of the status").DataType("string")).
		Param(ws.QueryParameter("page", "query the page number").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "query the page size number").DataType("integer")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.ListRevisionsResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListRevisionsResponse{}))

	ws.Route(ws.GET("/{appName}/revisions/{revision}").To(c.detailApplicationRevision).
		Doc("detail revision for application").
		Filter(c.RbacService.CheckPerm("revision", "detail")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application").DataType("string")).
		Param(ws.PathParameter("revision", "identifier of the application revision").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.DetailRevisionResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailRevisionResponse{}))

	ws.Route(ws.GET("/{appName}/envs").To(c.listApplicationEnvs).
		Doc("list policy for application").
		Filter(c.RbacService.CheckPerm("envBinding", "list")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.ListApplicationEnvBinding{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListApplicationEnvBinding{}))

	ws.Route(ws.POST("/{appName}/envs").To(c.createApplicationEnv).
		Doc("creating an application environment ").
		Filter(c.RbacService.CheckPerm("envBinding", "create")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Reads(apis.CreateApplicationEnvbindingRequest{}).
		Returns(200, "OK", apis.EnvBinding{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.PUT("/{appName}/envs/{envName}").To(c.updateApplicationEnv).
		Doc("set application  differences in the specified environment").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("envBinding", "update")).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the envBinding ").DataType("string")).
		Reads(apis.PutApplicationEnvBindingRequest{}).
		Returns(200, "OK", apis.EnvBinding{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EnvBinding{}))

	ws.Route(ws.DELETE("/{appName}/envs/{envName}").To(c.deleteApplicationEnv).
		Doc("delete an application environment ").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("envBinding", "delete")).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the envBinding ").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(404, "Not Found", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{appName}/envs/{envName}/status").To(c.getApplicationStatus).
		Doc("get application status").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("envBinding", "detail")).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Param(ws.PathParameter("envName", "identifier of the application envbinding").DataType("string")).
		Returns(200, "OK", apis.ApplicationStatusResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ApplicationStatusResponse{}))

	ws.Route(ws.POST("/{appName}/envs/{envName}/recycle").To(c.recycleApplicationEnv).
		Doc("get application status").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("envBinding", "recycle")).
		Filter(c.appCheckFilter).
		Filter(c.envCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string").Required(true)).
		Param(ws.PathParameter("envName", "identifier of the application envbinding").DataType("string").Required(true)).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{appName}/workflows").To(c.WorkflowAPI.listApplicationWorkflows).
		Doc("list application workflow").
		Filter(c.RbacService.CheckPerm("application/workflow", "list")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.ListWorkflowResponse{}).
		Writes(apis.ListWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.POST("/{appName}/workflows").To(c.WorkflowAPI.createOrUpdateApplicationWorkflow).
		Doc("create application workflow").
		Filter(c.RbacService.CheckPerm("application/workflow", "create")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateWorkflowRequest{}).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Returns(200, "create success", apis.DetailWorkflowResponse{}).
		Returns(400, "create failure", bcode.Bcode{}).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{appName}/workflows/{workflowName}").To(c.WorkflowAPI.detailWorkflow).
		Doc("detail application workflow").
		Filter(c.RbacService.CheckPerm("application/workflow", "detail")).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workfloc.").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Returns(200, "create success", apis.DetailWorkflowResponse{}).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{appName}/workflows/{workflowName}").To(c.WorkflowAPI.updateWorkflow).
		Doc("update application workflow config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application/workflow", "update")).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workflow").DataType("string")).
		Reads(apis.UpdateWorkflowRequest{}).
		Returns(200, "OK", apis.DetailWorkflowResponse{}).
		Writes(apis.DetailWorkflowResponse{}).Do(returns200, returns500))

	ws.Route(ws.DELETE("/{appName}/workflows/{workflowName}").To(c.WorkflowAPI.deleteWorkflow).
		Doc("deletet workflow").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application/workflow", "delete")).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workflow").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{appName}/workflows/{workflowName}/records").To(c.WorkflowAPI.listWorkflowRecords).
		Doc("query application workflow execution record").
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workflow").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application/workflow/record", "list")).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Param(ws.QueryParameter("page", "query the page number").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "query the page size number").DataType("integer")).
		Returns(200, "OK", apis.ListWorkflowRecordsResponse{}).
		Writes(apis.ListWorkflowRecordsResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{appName}/workflows/{workflowName}/records/{record}").To(c.WorkflowAPI.detailWorkflowRecord).
		Doc("query application workflow execution record detail").
		Filter(c.RbacService.CheckPerm("application/workflow/record", "detail")).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the workflow record").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Returns(200, "OK", apis.DetailWorkflowRecordResponse{}).
		Writes(apis.DetailWorkflowRecordResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{appName}/workflows/{workflowName}/records/{record}/resume").To(c.WorkflowAPI.resumeWorkflowRecord).
		Doc("resume suspend workflow record").
		Filter(c.RbacService.CheckPerm("application/workflow/record", "resume")).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the  workflow record").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Returns(200, "OK", nil).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailWorkflowRecordResponse{}))

	ws.Route(ws.GET("/{appName}/workflows/{workflowName}/records/{record}/terminate").To(c.WorkflowAPI.terminateWorkflowRecord).
		Doc("terminate suspend workflow record").
		Filter(c.RbacService.CheckPerm("application/workflow/record", "terminate")).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the workflow record").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Returns(200, "OK", nil).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailWorkflowRecordResponse{}))

	ws.Route(ws.GET("/{appName}/workflows/{workflowName}/records/{record}/rollback").To(c.WorkflowAPI.rollbackWorkflowRecord).
		Doc("rollback suspend application record").
		Filter(c.RbacService.CheckPerm("application/workflow/record", "rollback")).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Param(ws.PathParameter("workflowName", "identifier of the workflow").DataType("string")).
		Param(ws.PathParameter("record", "identifier of the workflow record").DataType("string")).
		Param(ws.QueryParameter("rollbackVersion", "identifier of the rollback revision").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Filter(c.WorkflowAPI.workflowCheckFilter).
		Returns(200, "OK", nil).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailWorkflowRecordResponse{}))

	ws.Route(ws.GET("/{appName}/records").To(c.listApplicationRecords).
		Doc("list application records").
		Filter(c.RbacService.CheckPerm("application/workflow/record", "list")).
		Param(ws.PathParameter("appName", "identifier of the application.").DataType("string").Required(true)).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.appCheckFilter).
		Returns(200, "OK", nil).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListWorkflowRecordsResponse{}))

	ws.Route(ws.POST("/{appName}/compare").To(c.compareAppWithLatestRevision).
		Doc("compare application with env latest revision").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "compare")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Returns(200, "OK", apis.ApplicationBase{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.AppCompareResponse{}))

	ws.Route(ws.POST("/{appName}/reset").To(c.resetAppToLatestRevision).
		Doc("reset application to latest revision").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "reset")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Returns(200, "OK", apis.AppResetResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.AppResetResponse{}))

	ws.Route(ws.POST("/{appName}/dry-run").To(c.dryRunAppOrRevision).
		Doc("dry-run application to latest revision").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("application", "detail")).
		Filter(c.appCheckFilter).
		Param(ws.PathParameter("appName", "identifier of the application ").DataType("string")).
		Returns(200, "OK", apis.AppDryRunResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.AppDryRunResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (c *applicationAPIInterface) createApplication(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateApplicationRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the domain layer code
	appBase, err := c.ApplicationService.CreateApplication(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create application failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(appBase); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) listApplications(req *restful.Request, res *restful.Response) {
	var projetNames []string
	if req.QueryParameter("project") != "" {
		projetNames = append(projetNames, req.QueryParameter("project"))
	}
	apps, err := c.ApplicationService.ListApplications(req.Request.Context(), apis.ListApplicationOptions{
		Projects:   projetNames,
		Env:        req.QueryParameter("env"),
		TargetName: req.QueryParameter("targetName"),
		Query:      req.QueryParameter("query"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationResponse{Applications: apps}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) detailApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.ApplicationService.DetailApplication(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) createApplicationTrigger(req *restful.Request, res *restful.Response) {
	var createReq apis.CreateApplicationTriggerRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	base, err := c.ApplicationService.CreateApplicationTrigger(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) listApplicationTriggers(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	triggers, err := c.ApplicationService.ListApplicationTriggers(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationTriggerResponse{Triggers: triggers}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) deleteApplicationTrigger(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	if err := c.ApplicationService.DeleteApplicationTrigger(req.Request.Context(), app, req.PathParameter("token")); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) publishApplicationTemplate(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	base, err := c.ApplicationService.PublishApplicationTemplate(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

// deployApplication TODO: return event model
func (c *applicationAPIInterface) deployApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.ApplicationDeployRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	deployRes, err := c.ApplicationService.Deploy(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(deployRes); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) deleteApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.ApplicationService.DeleteApplication(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) listApplicationComponents(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	components, err := c.ApplicationService.ListComponents(req.Request.Context(), app, apis.ListApplicationComponentOptions{
		EnvName: req.QueryParameter("envName"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ComponentListResponse{Components: components}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) createComponent(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.CreateComponentRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.ApplicationService.CreateComponent(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) detailComponent(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.ApplicationService.DetailComponent(req.Request.Context(), app, req.PathParameter("compName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) updateComponent(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	component := req.Request.Context().Value(&apis.CtxKeyApplicationComponent).(*model.ApplicationComponent)
	// Verify the validity of parameters
	var updateReq apis.UpdateApplicationComponentRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.ApplicationService.UpdateComponent(req.Request.Context(), app, component, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) deleteComponent(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	component := req.Request.Context().Value(&apis.CtxKeyApplicationComponent).(*model.ApplicationComponent)
	err := c.ApplicationService.DeleteComponent(req.Request.Context(), app, component)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) createApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.CreatePolicyRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.ApplicationService.CreatePolicy(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) listApplicationPolicies(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	policies, err := c.ApplicationService.ListPolicies(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationPolicy{Policies: policies}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) detailApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.ApplicationService.DetailPolicy(req.Request.Context(), app, req.PathParameter("policyName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) deleteApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.ApplicationService.DeletePolicy(req.Request.Context(), app, req.PathParameter("policyName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) updateApplicationPolicy(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var updateReq apis.UpdatePolicyRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	response, err := c.ApplicationService.UpdatePolicy(req.Request.Context(), app, req.PathParameter("policyName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(response); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) updateApplication(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var updateReq apis.UpdateApplicationRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.ApplicationService.UpdateApplication(req.Request.Context(), app, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) addApplicationTrait(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	var createReq apis.CreateApplicationTraitRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	trait, err := c.ApplicationService.CreateApplicationTrait(req.Request.Context(), app,
		&model.ApplicationComponent{Name: req.PathParameter("compName")}, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(trait); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) updateApplicationTrait(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	var updateReq apis.UpdateApplicationTraitRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	trait, err := c.ApplicationService.UpdateApplicationTrait(req.Request.Context(), app,
		&model.ApplicationComponent{Name: req.PathParameter("compName")}, req.PathParameter("traitType"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(trait); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) deleteApplicationTrait(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.ApplicationService.DeleteApplicationTrait(req.Request.Context(), app,
		&model.ApplicationComponent{Name: req.PathParameter("compName")}, req.PathParameter("traitType"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) getApplicationStatus(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	status, err := c.ApplicationService.GetApplicationStatus(req.Request.Context(), app, req.PathParameter("envName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(apis.ApplicationStatusResponse{Status: status, EnvName: req.PathParameter("envName")}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) listApplicationRevisions(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	revisions, err := c.ApplicationService.ListRevisions(req.Request.Context(), app.Name, req.QueryParameter("envName"), req.QueryParameter("status"), page, pageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(revisions); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) detailApplicationRevision(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.ApplicationService.DetailRevision(req.Request.Context(), app.Name, req.PathParameter("revision"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) updateApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var updateReq apis.PutApplicationEnvBindingRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	diff, err := c.EnvBindingService.UpdateEnvBinding(req.Request.Context(), app, req.PathParameter("envName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(diff); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) listApplicationEnvs(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	envBindings, err := c.EnvBindingService.GetEnvBindings(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListApplicationEnvBinding{EnvBindings: envBindings}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) createApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var createReq apis.CreateApplicationEnvbindingRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.EnvBindingService.CreateEnvBinding(req.Request.Context(), app, createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) deleteApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	err := c.EnvBindingService.DeleteEnvBinding(req.Request.Context(), app, req.PathParameter("envName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) appCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app, err := c.ApplicationService.GetApplication(req.Request.Context(), req.PathParameter("appName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplication, app))
	chain.ProcessFilter(req, res)
}

func (c *applicationAPIInterface) componentCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	component, err := c.ApplicationService.GetApplicationComponent(req.Request.Context(), app, req.PathParameter("compName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplicationComponent, component))
	chain.ProcessFilter(req, res)
}

func (c *applicationAPIInterface) envCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	envBinding, err := c.EnvBindingService.GetEnvBinding(req.Request.Context(), app, req.PathParameter("envName"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyApplicationEnvBinding, envBinding))
	chain.ProcessFilter(req, res)
}

func (c *applicationAPIInterface) applicationStatistics(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	detail, err := c.ApplicationService.Statistics(req.Request.Context(), app)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) recycleApplicationEnv(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	env := req.Request.Context().Value(&apis.CtxKeyApplicationEnvBinding).(*model.EnvBinding)
	err := c.EnvBindingService.ApplicationEnvRecycle(req.Request.Context(), app, env)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) listApplicationRecords(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	records, err := c.ApplicationService.ListRecords(req.Request.Context(), app.Name)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(records); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) compareAppWithLatestRevision(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var compareReq apis.AppCompareReq
	if err := req.ReadEntity(&compareReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&compareReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	base, err := c.ApplicationService.CompareAppWithLatestRevision(req.Request.Context(), app, compareReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) resetAppToLatestRevision(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)

	base, err := c.ApplicationService.ResetAppToLatestRevision(req.Request.Context(), app.Name)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *applicationAPIInterface) dryRunAppOrRevision(req *restful.Request, res *restful.Response) {
	app := req.Request.Context().Value(&apis.CtxKeyApplication).(*model.Application)
	// Verify the validity of parameters
	var dryRunReq apis.AppDryRunReq
	if err := req.ReadEntity(&dryRunReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&dryRunReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if dryRunReq.AppName == "" {
		dryRunReq.AppName = app.Name
	}

	base, err := c.ApplicationService.DryRunAppOrRevision(req.Request.Context(), app, dryRunReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
