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
	restful "github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type payloadTypesWebservice struct {
}

func (c *payloadTypesWebservice) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/payload_types").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for payload types manage")

	tags := []string{"payload_types"}

	ws.Route(ws.GET("/").To(c.ListPayloadTypes).
		Doc("list application trigger payload types").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Returns(200, "OK", nil).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	return ws
}

func (c *payloadTypesWebservice) ListPayloadTypes(req *restful.Request, res *restful.Response) {
	if err := res.WriteEntity(usecase.WebhookHandlers); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
