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

package api

import (
	"context"
	"strconv"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils"
)

type helmAPIInterface struct {
	HelmService service.HelmService `inject:""`
}

// NewHelmAPIInterface will return helm APIInterface
func NewHelmAPIInterface() Interface {
	return &helmAPIInterface{}
}

func (h helmAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/repository").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for helm")

	tags := []string{"repository", "helm"}

	// List charts
	ws.Route(ws.GET("/chart_repos").To(h.listRepo).
		Doc("list chart repo").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("project", "the config project").DataType("string")).
		Returns(200, "OK", []string{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	// List charts
	ws.Route(ws.GET("/charts").To(h.listCharts).
		Doc("list charts").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("repoUrl", "helm repository url").DataType("string")).
		Param(ws.QueryParameter("secretName", "secret of the repo").DataType("string")).
		Returns(200, "OK", []string{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	// List available chart versions
	ws.Route(ws.GET("/charts/{chart}/versions").To(h.listVersions).
		Doc("list versions").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("repoUrl", "helm repository url").DataType("string")).
		Param(ws.QueryParameter("secretName", "secret of the repo").DataType("string")).
		Returns(200, "OK", v1.ChartVersionListResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	// List available chart versions
	ws.Route(ws.GET("/charts/{chart}/versions/{version}/values").To(h.chartValues).
		Doc("get chart value").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Param(ws.QueryParameter("repoUrl", "helm repository url").DataType("string")).
		Param(ws.QueryParameter("secretName", "secret of the repo").DataType("string")).
		Returns(200, "OK", map[string]interface{}{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes([]string{}))

	ws.Filter(authCheckFilter)
	return ws
}

func (h helmAPIInterface) listCharts(req *restful.Request, res *restful.Response) {
	url := utils.Sanitize(req.QueryParameter("repoUrl"))
	secName := utils.Sanitize(req.QueryParameter("secretName"))
	skipCache, err := isSkipCache(req)
	if err != nil {
		bcode.ReturnError(req, res, bcode.ErrSkipCacheParameter)
		return
	}
	charts, err := h.HelmService.ListChartNames(context.Background(), url, secName, skipCache)
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

func (h helmAPIInterface) listVersions(req *restful.Request, res *restful.Response) {
	url := req.QueryParameter("repoUrl")
	chartName := req.PathParameter("chart")
	secName := req.QueryParameter("secretName")
	skipCache, err := isSkipCache(req)
	if err != nil {
		bcode.ReturnError(req, res, bcode.ErrSkipCacheParameter)
		return
	}

	versions, err := h.HelmService.ListChartVersions(context.Background(), url, chartName, secName, skipCache)
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

func (h helmAPIInterface) chartValues(req *restful.Request, res *restful.Response) {
	url := req.QueryParameter("repoUrl")
	secName := req.QueryParameter("secretName")
	chartName := req.PathParameter("chart")
	version := req.PathParameter("version")
	skipCache, err := isSkipCache(req)
	if err != nil {
		bcode.ReturnError(req, res, bcode.ErrSkipCacheParameter)
		return
	}

	versions, err := h.HelmService.GetChartValues(context.Background(), url, chartName, version, secName, skipCache)
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

func (h helmAPIInterface) listRepo(req *restful.Request, res *restful.Response) {
	project := req.QueryParameter("project")
	repos, err := h.HelmService.ListChartRepo(context.Background(), project)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	err = res.WriteEntity(repos)
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
