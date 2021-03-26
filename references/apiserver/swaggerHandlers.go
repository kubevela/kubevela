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
	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
)

// SwaggerJSON use /swagger.json and /doc.json 404
func (s *APIServer) SwaggerJSON(c *gin.Context) {
	path := c.Param("any")
	switch path {
	case "/doc.json":
		c.String(404, "404 page not found")
	case "/swagger.json":
		c.Header("Content-Type", "application/json")
		doc, err := swag.ReadDoc()
		if err != nil {
			panic(err)
		}
		c.String(200, doc)
	default:
		return
	}
	c.Abort()
}
