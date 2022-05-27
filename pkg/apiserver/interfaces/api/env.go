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
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

type envAPIInterface struct {
	EnvService         service.EnvService         `inject:""`
	ApplicationService service.ApplicationService `inject:""`
	RBACService        service.RBACService        `inject:""`
}

// NewEnvAPIInterface new env APIInterface
func NewEnvAPIInterface() Interface {
	return &envAPIInterface{}
}

func (n *envAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/envs").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for env management")

	tags := []string{"env"}

	ws.Route(ws.GET("/").To(n.list).
		Operation("envlist").
		Doc("list all envs").
		// This api will filter the environments by user's permissions
		// Filter(n.RbacService.CheckPerm("environment", "list")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.ListEnvResponse{}).
		Writes(apis.ListEnvResponse{}))

	ws.Route(ws.POST("/").To(n.create).
		Operation("envcreate").
		Doc("create an env").
		Filter(n.RBACService.CheckPerm("environment", "create")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateEnvRequest{}).
		Returns(200, "OK", apis.Env{}).
		Writes(apis.Env{}))

	ws.Route(ws.PUT("/{envName}").To(n.update).
		Operation("envupdate").
		Doc("update an env").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RBACService.CheckPerm("environment", "update")).
		Param(ws.PathParameter("envName", "identifier of the environment").DataType("string")).
		Reads(apis.CreateEnvRequest{}).
		Returns(200, "OK", apis.Env{}).
		Writes(apis.Env{}))

	ws.Route(ws.DELETE("/{envName}").To(n.delete).
		Operation("envdelete").
		Doc("delete one env").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(n.RBACService.CheckPerm("environment", "delete")).
		Param(ws.PathParameter("envName", "identifier of the environment").DataType("string")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (n *envAPIInterface) list(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	project := req.QueryParameter("project")
	envs, err := n.EnvService.ListEnvs(req.Request.Context(), page, pageSize, apis.ListEnvOptions{Project: project})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(envs); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

// it will prevent the deletion if there's still application in it.
func (n *envAPIInterface) delete(req *restful.Request, res *restful.Response) {
	envname := req.PathParameter("envName")

	ctx := req.Request.Context()
	lists, err := n.ApplicationService.ListApplications(ctx, apis.ListApplicationOptions{Env: envname})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if len(lists) > 0 {
		log.Logger.Infof("detected %d applications in this env, the first is %s", len(lists), lists[0].Name)
		bcode.ReturnError(req, res, bcode.ErrDeleteEnvButAppExist)
		return
	}

	err = n.EnvService.DeleteEnv(ctx, envname)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err = res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *envAPIInterface) create(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateEnvRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the Domain layer code
	env, err := n.EnvService.CreateEnv(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create application failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(env); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *envAPIInterface) update(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var updateReq apis.UpdateEnvRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	env, err := n.EnvService.UpdateEnv(req.Request.Context(), req.PathParameter("envName"), updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err := res.WriteEntity(env); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
