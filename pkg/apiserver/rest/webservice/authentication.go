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
	"strings"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type authenticationWebService struct {
	authenticationUsecase usecase.AuthenticationUsecase
	userUsecase           usecase.UserUsecase
}

// NewAuthenticationWebService is the webservice of authentication
func NewAuthenticationWebService(authenticationUsecase usecase.AuthenticationUsecase, userUsecase usecase.UserUsecase) WebService {
	return &authenticationWebService{
		authenticationUsecase: authenticationUsecase,
		userUsecase:           userUsecase,
	}
}

func (c *authenticationWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/auth").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for authentication manage")

	tags := []string{"authentication"}

	ws.Route(ws.POST("/login").To(c.login).
		Doc("handle login request").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.LoginRequest{}).
		Returns(200, "", apis.LoginResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.LoginResponse{}))

	ws.Route(ws.GET("/dex_config").To(c.getDexConfig).
		Doc("get Dex config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.DexConfigResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.DexConfigResponse{}))

	ws.Route(ws.GET("/refresh_token").To(c.refreshToken).
		Doc("refresh token").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.RefreshTokenResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.RefreshTokenResponse{}))

	ws.Route(ws.GET("/login_type").To(c.getLoginType).
		Doc("get login type").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.GetLoginTypeResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.GetLoginTypeResponse{}))

	ws.Route(ws.GET("/user_info").To(c.getLoginUserInfo).
		Doc("get login user detail info").
		Filter(authCheckFilter).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.LoginUserInfoResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.LoginUserInfoResponse{}))
	return ws
}

func authCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	tokenHeader := req.HeaderParameter("Authorization")
	if tokenHeader == "" {
		bcode.ReturnError(req, res, bcode.ErrNotAuthorized)
		return
	}
	splitted := strings.Split(tokenHeader, " ")
	if len(splitted) != 2 {
		bcode.ReturnError(req, res, bcode.ErrNotAuthorized)
		return
	}

	token, err := usecase.ParseToken(splitted[1])
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if token.GrantType != usecase.GrantTypeAccess {
		bcode.ReturnError(req, res, bcode.ErrNotAccessToken)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyUser, token.Username))

	chain.ProcessFilter(req, res)
}

func (c *authenticationWebService) login(req *restful.Request, res *restful.Response) {
	var loginReq apis.LoginRequest
	if err := req.ReadEntity(&loginReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	base, err := c.authenticationUsecase.Login(req.Request.Context(), loginReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *authenticationWebService) getDexConfig(req *restful.Request, res *restful.Response) {
	base, err := c.authenticationUsecase.GetDexConfig(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *authenticationWebService) refreshToken(req *restful.Request, res *restful.Response) {
	base, err := c.authenticationUsecase.RefreshToken(req.Request.Context(), req.HeaderParameter("RefreshToken"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *authenticationWebService) getLoginType(req *restful.Request, res *restful.Response) {
	base, err := c.authenticationUsecase.GetLoginType(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *authenticationWebService) getLoginUserInfo(req *restful.Request, res *restful.Response) {
	info, err := c.userUsecase.DetailLoginUserInfo(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(info); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
