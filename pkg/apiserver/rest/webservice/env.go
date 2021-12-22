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
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type envWebService struct {
	envUsecase usecase.EnvUsecase
	appUsecase usecase.ApplicationUsecase
}

// NewEnvWebService new env webservice
func NewEnvWebService(envUsecase usecase.EnvUsecase, appUseCase usecase.ApplicationUsecase) WebService {
	return &envWebService{envUsecase: envUsecase, appUsecase: appUseCase}
}

func (n *envWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/envs").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for env management")

	tags := []string{"env"}

	ws.Route(ws.GET("/").To(n.list).
		Doc("list all envs").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.ListEnvResponse{}).
		Writes(apis.ListEnvResponse{}))

	ws.Route(ws.POST("/").To(n.create).
		Doc("create an env").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateEnvRequest{}).
		Returns(200, "", apis.Env{}).
		Writes(apis.Env{}))

	ws.Route(ws.PUT("/").To(n.update).
		Doc("create an env").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateEnvRequest{}).
		Returns(200, "", apis.Env{}).
		Writes(apis.Env{}))

	ws.Route(ws.DELETE("/{name}").To(n.delete).
		Doc("delete one env").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))
	return ws
}

func (n *envWebService) list(req *restful.Request, res *restful.Response) {
	envs, err := n.envUsecase.ListEnvs(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.ListEnvResponse{Envs: envs}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

// it will prevent the deletion if there's still application in it.
func (n *envWebService) delete(req *restful.Request, res *restful.Response) {
	envname := req.PathParameter("name")

	ctx := req.Request.Context()
	lists, err := n.appUsecase.ListApplications(ctx, apis.ListApplicatioOptions{Env: envname})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if len(lists) > 0 {
		bcode.ReturnError(req, res, bcode.ErrDeleteEnvButAppExist)
		return
	}

	err = n.envUsecase.DeleteEnv(ctx, envname)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err = res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (n *envWebService) create(req *restful.Request, res *restful.Response) {
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
	// Call the usecase layer code
	env, err := n.envUsecase.CreateEnv(req.Request.Context(), createReq)
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

func (n *envWebService) update(req *restful.Request, res *restful.Response) {
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

	env, err := n.envUsecase.UpdateEnv(req.Request.Context(), updateReq)
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
