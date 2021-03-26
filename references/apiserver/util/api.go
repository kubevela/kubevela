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

package util

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/references/apiserver/apis"
)

// DefaultDashboardPort refers to the default port number of dashboard
var DefaultDashboardPort = ":38081"

// DefaultAPIServerPort refers to the default port number of APIServer
const DefaultAPIServerPort = ":38081"

// AssembleResponse assembles response data to return
func AssembleResponse(c *gin.Context, data interface{}, err error) {
	var code = http.StatusOK
	if err != nil {
		code = http.StatusInternalServerError
		c.JSON(code, apis.Response{
			Code: code,
			Data: err.Error(),
		})
		return
	}

	c.JSON(code, apis.Response{
		Code: code,
		Data: data,
	})
}

// URL returns the URL of Dashboard based on default port
func URL(url string) string {
	return fmt.Sprintf("http://127.0.0.1%s/api%s", DefaultDashboardPort, url)
}
