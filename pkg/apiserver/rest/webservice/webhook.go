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

type webhookWebService struct {
	webhookUsecase     usecase.WebhookUsecase
	applicationUsecase usecase.ApplicationUsecase
}

// NewWebhookWebService new application manage webservice
func NewWebhookWebService(webhookUsecase usecase.WebhookUsecase, applicationUsecase usecase.ApplicationUsecase) WebService {
	return &webhookWebService{
		webhookUsecase:     webhookUsecase,
		applicationUsecase: applicationUsecase,
	}
}

func (c *webhookWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/webhook").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for webhook manage")

	tags := []string{"webhook"}

	ws.Route(ws.POST("/{token}").To(c.handleApplicationWebhook).
		Doc("handle application webhook request").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.PathParameter("name", "identifier of the application ").DataType("string")).
		Reads(apis.HandleApplicationTriggerWebhookRequest{}).
		Returns(200, "", apis.ApplicationDeployResponse{}).
		Returns(400, "", bcode.Bcode{}).
		Writes(apis.ApplicationDeployResponse{}))
	return ws
}

func (c *webhookWebService) handleApplicationWebhook(req *restful.Request, res *restful.Response) {
	base, err := c.webhookUsecase.HandleApplicationWebhook(req.Request.Context(), req.PathParameter("token"), req)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(base); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
