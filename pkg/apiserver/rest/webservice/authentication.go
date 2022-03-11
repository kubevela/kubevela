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
	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type authenticationWebService struct {
	authenticationUsecase usecase.AuthenticationUsecase
}

// NewAuthenticationWebService is the webservice of authentication
func NewAuthenticationWebService(authenticationUsecase usecase.AuthenticationUsecase) WebService {
	return &authenticationWebService{
		authenticationUsecase: authenticationUsecase,
	}
}

func (c *authenticationWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix).
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for authentication manage")

	tags := []string{"authentication"}

	ws.Route(ws.GET("/login").To(c.login).
		Doc("handle login request").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "", apis.LoginResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.LoginResponse{}))
	return ws
}

func (c *authenticationWebService) login(req *restful.Request, res *restful.Response) {
	base, err := c.authenticationUsecase.Login(req.Request.Context(), req)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
