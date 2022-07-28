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

package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/gorilla/websocket"
	"github.com/koding/websocketproxy"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

// CloudShellAPIInterface provide the API for preparing the cloud shell environment
type CloudShellAPIInterface struct {
	RbacService       service.RBACService       `inject:""`
	CloudShellService service.CloudShellService `inject:""`
}

// NewCloudShellAPIInterface create the cloudshell api instance
func NewCloudShellAPIInterface() *CloudShellAPIInterface {
	return &CloudShellAPIInterface{}
}

// GetWebServiceRoute -
func (c *CloudShellAPIInterface) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(versionPrefix+"/cloudshell").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for cloudshell manage")

	tags := []string{"cloudshell"}

	ws.Route(ws.POST("/").To(c.prepareCloudShell).
		Doc("prepare the user's cloud shell environment").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("cloudshell", "create")).
		Returns(200, "OK", apis.CloudShellPrepareResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.CloudShellPrepareResponse{}).Do(returns200, returns500))

	ws.Route(ws.DELETE("/").To(c.destroyCloudShell).
		Doc("destroy the user's cloud shell environment").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("cloudshell", "delete")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	ws.Filter(authCheckFilter)
	return ws
}

func (c *CloudShellAPIInterface) prepareCloudShell(req *restful.Request, res *restful.Response) {
	prepare, err := c.CloudShellService.Prepare(req.Request.Context())
	// Write back response data
	if err != nil {
		if prepare == nil {
			bcode.ReturnError(req, res, err)
			return
		}
		prepare.Status = service.StatusFailed
		prepare.Message = err.Error()
	}
	if err := res.WriteEntity(prepare); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

func (c *CloudShellAPIInterface) destroyCloudShell(req *restful.Request, res *restful.Response) {
	err := c.CloudShellService.Destroy(req.Request.Context())
	// Write back response data
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if err := res.WriteEntity(apis.EmptyResponse{}); err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
}

// CloudShellView provide the view handler
type CloudShellView struct {
	RbacService       service.RBACService       `inject:""`
	CloudShellService service.CloudShellService `inject:""`
}

// NewCloudShellView new cloud share
func NewCloudShellView() *CloudShellView {
	return &CloudShellView{}
}

// GetWebServiceRoute -
func (c *CloudShellView) GetWebServiceRoute() *restful.WebService {
	ws := new(restful.WebService)
	ws.Path(viewPrefix+"/cloudshell").
		Consumes(restful.MIME_XML, restful.MIME_JSON).
		Produces(restful.MIME_JSON, restful.MIME_XML).
		Doc("api for cluster manage")

	tags := []string{"cloudshell"}

	ws.Route(ws.GET("/").To(c.proxy).
		Doc("prepare the user's cloud shell environment").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("cloudshell", "create")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	ws.Route(ws.GET("/{subpath:*}").To(c.proxy).
		Doc("prepare the user's cloud shell environment").
		Metadata(restfulspec.KeyOpenAPITags, tags).
		Filter(c.RbacService.CheckPerm("cloudshell", "create")).
		Returns(200, "OK", apis.EmptyResponse{}).
		Returns(400, "Bad Request", bcode.Bcode{}).
		Writes(apis.EmptyResponse{}).Do(returns200, returns500))

	ws.Filter(authCheckFilter)
	return ws
}

func (c *CloudShellView) proxy(req *restful.Request, res *restful.Response) {
	endpoint, err := c.CloudShellService.GetCloudShellEndpoint(req.Request.Context())
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	if req.HeaderParameter("Upgrade") == "websocket" && req.HeaderParameter("Connection") == "Upgrade" {
		u, err := url.Parse("ws://" + endpoint)
		if err != nil {
			bcode.ReturnError(req, res, err)
			return
		}
		req.Request.URL.Path = strings.Replace(req.Request.URL.Path, "/view/cloudshell", "", 1)
		// proxy the websocket request
		proxy := websocketproxy.NewProxy(u)
		proxy.Upgrader = &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(req *http.Request) bool {
				return true
			},
		}
		proxy.ServeHTTP(res.ResponseWriter, req.Request)
		return
	}
	u, err := url.Parse("http://" + endpoint)
	if err != nil {
		bcode.ReturnError(req, res, err)
		return
	}
	NewReverseProxy(u).ServeHTTP(res.ResponseWriter, req.Request)
}

// NewReverseProxy proxy for requests of the cloud shell
func NewReverseProxy(target *url.URL) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = strings.Replace(req.URL.Path, "/view/cloudshell", "", 1)
	}
	return &httputil.ReverseProxy{Director: director}
}
