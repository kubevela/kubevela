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

package apiserver

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/version"
)

// GetVersion will return version for dashboard
func (s *APIServer) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, apis.Response{
		Code: http.StatusOK,
		Data: map[string]string{"version": version.VelaVersion},
	})
}
