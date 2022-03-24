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

	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type systemInfoWebService struct {
	useCase usecase.SystemInfoUsecase
}

// NewSystemInfoWebService return systemInfo webservice
func NewSystemInfoWebService(systemInfoUseCase usecase.SystemInfoUsecase) WebService {
	return &systemInfoWebService{useCase: systemInfoUseCase}
}

// GetWebService return systemInfo webservice
func (u systemInfoWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/system_info").Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for systemInfo management")

	tags := []string{"systemInfo"}

	// Get
	ws.Route(ws.GET("/").To(u.getSystemInfo).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.SystemInfoResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.SystemInfoResponse{}))

	// Delete
	ws.Route(ws.DELETE("/").To(u.deleteSystemInfo).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", apis.SystemInfoResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.SystemInfoResponse{}))

	// Post
	ws.Route(ws.PUT("/").To(u.updateSystemInfo).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.SystemInfoRequest{}).
		Returns(200, "OK", apis.SystemInfoResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.SystemInfoResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (u systemInfoWebService) getSystemInfo(req *restful.Request, res *restful.Response) {
	info, err := u.useCase.GetSystemInfo(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(info); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (u systemInfoWebService) updateSystemInfo(req *restful.Request, res *restful.Response) {
	var systemInfoReq apis.SystemInfoRequest
	var args []byte
	_, err := req.Request.Body.Read(args)
	if err == nil {
		err := req.ReadEntity(&systemInfoReq)
		if err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
		if err = validate.Struct(&systemInfoReq); err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
	}

	info, err := u.useCase.UpdateSystemInfo(req.Request.Context(), systemInfoReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(info); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (u systemInfoWebService) deleteSystemInfo(req *restful.Request, res *restful.Response) {
	err := u.useCase.DeleteSystemInfo(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
