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
	"strconv"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/usecase"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

type helmWebService struct {
	usecase usecase.HelmHandler
}

// NewHelmWebService will return helm webService
func NewHelmWebService(u usecase.HelmHandler) WebService {
	return helmWebService{usecase: u}
}

func (h helmWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/repository").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for helm")

	tags := []string{"repository", "helm"}

	// List charts
	ws.Route(ws.GET("/charts").To(h.listCharts).
		Doc("list charts").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("repoUrl", "helm repository url").DataType("string")).
		Returns(200, "OK", []string{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	// List available chart versions
	ws.Route(ws.GET("/charts/{chart}/versions").To(h.listVersions).
		Doc("list versions").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("repoUrl", "helm repository url").DataType("string")).
		Returns(200, "OK", v1.ChartVersionListResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	// List available chart versions
	ws.Route(ws.GET("/charts/{chart}/versions/{version}/values").To(h.chartValues).
		Doc("get chart value").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("repoUrl", "helm repository url").DataType("string")).
		Returns(200, "OK", map[string]interface{}{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (h helmWebService) listCharts(req *restful.Request, res *restful.Response) {
	url := req.QueryParameter("repoUrl")
	skipCache, err := isSkipCache(req)
	if err != nil {
		bcode.ReturnError(req, res, bcode.ErrSkipCacheParameter)
		return
	}
	charts, err := h.usecase.ListChartNames(context.Background(), url, skipCache)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(charts)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (h helmWebService) listVersions(req *restful.Request, res *restful.Response) {
	url := req.QueryParameter("repoUrl")
	chartName := req.PathParameter("chart")
	skipCache, err := isSkipCache(req)
	if err != nil {
		bcode.ReturnError(req, res, bcode.ErrSkipCacheParameter)
		return
	}

	versions, err := h.usecase.ListChartVersions(context.Background(), url, chartName, skipCache)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(v1.ChartVersionListResponse{Versions: versions})
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (h helmWebService) chartValues(req *restful.Request, res *restful.Response) {
	url := req.QueryParameter("repoUrl")
	chartName := req.PathParameter("chart")
	version := req.PathParameter("version")
	skipCache, err := isSkipCache(req)
	if err != nil {
		bcode.ReturnError(req, res, bcode.ErrSkipCacheParameter)
		return
	}

	versions, err := h.usecase.GetChartValues(context.Background(), url, chartName, version, skipCache)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(versions)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func isSkipCache(req *restful.Request) (bool, error) {
	skipStr := req.QueryParameter("skipCache")
	skipCache := false
	var err error
	if skipStr != "" {
		if skipCache, err = strconv.ParseBool(skipStr); err != nil {
			return skipCache, err
		}
	}
	return skipCache, nil
}
