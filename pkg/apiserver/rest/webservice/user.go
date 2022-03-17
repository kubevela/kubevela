/*
Copyright 2022 The KubeVela Authors.

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

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type userWebService struct {
	userUsecase usecase.UserUsecase
}

// NewUserWebService is the webservice of user
func NewUserWebService(userUsecase usecase.UserUsecase) WebService {
	return &userWebService{
		userUsecase: userUsecase,
	}
}

func (c *userWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/users").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for user manage")

	tags := []string{"users"}

	ws.Route(ws.GET("/").To(c.listUser).
		Doc("list users").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("page", "query the page number").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "query the page size number").DataType("integer")).
		Param(ws.QueryParameter("name", "fuzzy search based on name").DataType("string")).
		Param(ws.QueryParameter("email", "fuzzy search based on email").DataType("string")).
		Param(ws.QueryParameter("alias", "fuzzy search based on alias").DataType("string")).
		Returns(200, "OK", apis.ListUserResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.ListUserResponse{}))

	ws.Route(ws.POST("/").To(c.createUser).
		Doc("create a user").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateUserRequest{}).
		Returns(200, "OK", apis.DetailUserResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailUserResponse{}))

	ws.Route(ws.GET("/{username}").To(c.detailUser).
		Doc("get user detail").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.userCheckFilter).
		Returns(200, "OK", apis.DetailUserResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailUserResponse{}))

	ws.Route(ws.PUT("/{username}").To(c.updateUser).
		Doc("update a user's alias or password").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.userCheckFilter).
		Returns(200, "OK", apis.DetailUserResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.DetailUserResponse{}))

	ws.Route(ws.DELETE("/{username}").To(c.deleteUser).
		Doc("delete a user").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{username}/disable").To(c.disableUser).
		Doc("disable a user").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.userCheckFilter).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))

	ws.Route(ws.GET("/{username}/enable").To(c.enableUser).
		Doc("enable a user").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.userCheckFilter).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}))
	return ws
}

func (c *userWebService) userCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	user, err := c.userUsecase.GetUser(req.Request.Context(), req.PathParameter("username"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyUser, user))
	chain.ProcessFilter(req, res)
}

func (c *userWebService) createUser(req *restful.Request, res *restful.Response) {
	var createReq apis.CreateUserRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	resp, err := c.userUsecase.CreateUser(req.Request.Context(), createReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *userWebService) detailUser(req *restful.Request, res *restful.Response) {
	user := req.Request.Context().Value(&apis.CtxKeyUser).(*model.User)
	resp, err := c.userUsecase.DetailUser(req.Request.Context(), user)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *userWebService) deleteUser(req *restful.Request, res *restful.Response) {
	err := c.userUsecase.DeleteUser(req.Request.Context(), req.PathParameter("username"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *userWebService) listUser(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	resp, err := c.userUsecase.ListUsers(req.Request.Context(), page, pageSize, apis.ListUserOptions{
		Name:  req.QueryParameter("name"),
		Alias: req.QueryParameter("alias"),
		Email: req.QueryParameter("email"),
	})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *userWebService) updateUser(req *restful.Request, res *restful.Response) {
	user := req.Request.Context().Value(&apis.CtxKeyUser).(*model.User)
	var updateReq apis.UpdateUserRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	resp, err := c.userUsecase.UpdateUser(req.Request.Context(), user, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(resp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *userWebService) disableUser(req *restful.Request, res *restful.Response) {
	user := req.Request.Context().Value(&apis.CtxKeyUser).(*model.User)
	err := c.userUsecase.DisableUser(req.Request.Context(), user)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *userWebService) enableUser(req *restful.Request, res *restful.Response) {
	user := req.Request.Context().Value(&apis.CtxKeyUser).(*model.User)
	err := c.userUsecase.EnableUser(req.Request.Context(), user)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
