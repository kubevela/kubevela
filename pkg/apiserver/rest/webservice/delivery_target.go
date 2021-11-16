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

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	restful "github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// NewDeliveryTargetWebService new deliveryTarget webservice
func NewDeliveryTargetWebService(deliveryTargetUsecase usecase.DeliveryTargetUsecase) WebService {
	return &DeliveryTargetWebService{
		deliveryTargetUsecase: deliveryTargetUsecase,
	}
}

// DeliveryTargetWebService delivery target web service
type DeliveryTargetWebService struct {
	deliveryTargetUsecase usecase.DeliveryTargetUsecase
}

// GetWebService get web service
func (dt *DeliveryTargetWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/deliveryTargets").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for deliveryTarget manage")

	tags := []string{"deliveryTarget"}

	ws.Route(ws.GET("/").To(dt.listDeliveryTargets).
		Doc("list deliveryTarget").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("namesapce", "Query the delivery target belonging to a namespace").DataType("string")).
		Param(ws.QueryParameter("page", "Page for paging").DataType("integer")).
		Param(ws.QueryParameter("pageSize", "PageSize for paging").DataType("integer")).
		Returns(200, "", apis.ListDeliveryTargetResponse{}).
		Writes(apis.ListDeliveryTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.POST("/").To(dt.createDeliveryTarget).
		Doc("create deliveryTarget").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Reads(apis.CreateDeliveryTargetRequest{}).
		Returns(200, "create success", apis.DetailDeliveryTargetResponse{}).
		Returns(400, "create failure", bcode.Bcode{}).
		Writes(apis.DetailDeliveryTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{name}").To(dt.detailDeliveryTarget).
		Doc("detail deliveryTarget").
		Param(ws.PathParameter("name", "identifier of the deliveryTarget.").DataType("string")).
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.deliveryTargetCheckFilter).
		Returns(200, "create success", apis.DetailDeliveryTargetResponse{}).
		Writes(apis.DetailDeliveryTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.PUT("/{name}").To(dt.updateDeliveryTarget).
		Doc("update application DeliveryTarget config").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.deliveryTargetCheckFilter).
		Param(ws.PathParameter("name", "identifier of the deliveryTarget").DataType("string")).
		Reads(apis.UpdateDeliveryTargetRequest{}).
		Returns(200, "", apis.DetailDeliveryTargetResponse{}).
		Writes(apis.DetailDeliveryTargetResponse{}).Do(returns200, returns500))

	ws.Route(ws.DELETE("/{name}").To(dt.deleteDeliveryTarget).
		Doc("deletet DeliveryTarget").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(dt.deliveryTargetCheckFilter).
		Param(ws.PathParameter("name", "identifier of the deliveryTarget").DataType("string")).
		Returns(200, "", apis.EmptyResponse{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	return ws
}

func (dt *DeliveryTargetWebService) createDeliveryTarget(req *restful.Request, res *restful.Response) {
	// Verify the validity of parameters
	var createReq apis.CreateDeliveryTargetRequest
	if err := req.ReadEntity(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&createReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	// Call the usecase layer code
	deliveryTargetDetail, err := dt.deliveryTargetUsecase.CreateDeliveryTarget(req.Request.Context(), createReq)
	if err != nil {
		log.Logger.Errorf("create delivery-target failure %s", err.Error())
		bcode.ReturnError(req, res, err)
		return
	}
	// Write back response data
	if err := res.WriteEntity(deliveryTargetDetail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *DeliveryTargetWebService) deliveryTargetCheckFilter(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	deliveryTarget, err := dt.deliveryTargetUsecase.GetDeliveryTarget(req.Request.Context(), req.PathParameter("name"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	req.Request = req.Request.WithContext(context.WithValue(req.Request.Context(), &apis.CtxKeyDeliveryTarget, deliveryTarget))
	chain.ProcessFilter(req, res)
}

func (dt *DeliveryTargetWebService) detailDeliveryTarget(req *restful.Request, res *restful.Response) {
	deliveryTarget := req.Request.Context().Value(&apis.CtxKeyDeliveryTarget).(*model.DeliveryTarget)
	detail, err := dt.deliveryTargetUsecase.DetailDeliveryTarget(req.Request.Context(), deliveryTarget)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *DeliveryTargetWebService) updateDeliveryTarget(req *restful.Request, res *restful.Response) {
	deliveryTarget := req.Request.Context().Value(&apis.CtxKeyDeliveryTarget).(*model.DeliveryTarget)
	// Verify the validity of parameters
	var updateReq apis.UpdateDeliveryTargetRequest
	if err := req.ReadEntity(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := validate.Struct(&updateReq); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	detail, err := dt.deliveryTargetUsecase.UpdateDeliveryTarget(req.Request.Context(), deliveryTarget, updateReq)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(detail); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *DeliveryTargetWebService) deleteDeliveryTarget(req *restful.Request, res *restful.Response) {
	if err := dt.deliveryTargetUsecase.DeleteDeliveryTarget(req.Request.Context(), req.PathParameter("name")); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (dt *DeliveryTargetWebService) listDeliveryTargets(req *restful.Request, res *restful.Response) {
	page, pageSize, err := utils.ExtractPagingParams(req, minPageSize, maxPageSize)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	deliveryTargets, err := dt.deliveryTargetUsecase.ListDeliveryTargets(req.Request.Context(), page, pageSize, req.QueryParameter("namespace"))
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	if err := res.WriteEntity(deliveryTargets); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
