/*
 Copyright 2021. The KubeVela Authors.

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
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

type velaQLAPIInterface struct {
	VelaQLService service.VelaQLService `inject:""`
	RbacService   service.RBACService   `inject:""`
}

// NewVelaQLAPIInterface new velaQL APIInterface
func NewVelaQLAPIInterface() Interface {
	return &velaQLAPIInterface{}
}

func (v *velaQLAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/query").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for velaQL")

	tags := []string{"velaQL"}

	ws.Route(ws.GET("/").To(v.queryView).
		Doc("use velaQL to query resource status").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		// TODO: VelaQL is an open data query API that is currently not compatible with RBAC.
		// Filter(v.RbacService.CheckPerm("application", "detail")).
		Param(ws.QueryParameter("velaql", "velaql query statement").DataType("string")).
		Returns(200, "OK", apis.VelaQLViewResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.VelaQLViewResponse{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (v *velaQLAPIInterface) queryView(req *restful.Request, res *restful.Response) {
	velaQL := req.QueryParameter("velaql")

	qlResp, err := v.VelaQLService.QueryView(req.Request.Context(), velaQL)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}

	// Write back response data
	if err = res.WriteEntity(qlResp); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}
