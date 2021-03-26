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
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/references/apiserver/util"
	"github.com/oam-dev/kubevela/references/common"
)

// GetDefinition gets OpenAPI schema from Cue section of a WorkloadDefinition/TraitDefinition
// @tags definitions
// @ID GetDefinition
// @Summary gets OpenAPI schema from Cue section of a WorkloadDefinition/TraitDefinition
// @Param definitionName path string true "name of workload type or trait"
// @Success 200 {object} apis.Response{code=int,data=string}
// @Failure 500 {object} apis.Response{code=int,data=string}
// @Router /definitions/{definitionName} [get]
func (s *APIServer) GetDefinition(c *gin.Context) {
	definitionName := c.Param("name")
	cm, err := common.GetCapabilityConfigMap(s.KubeClient, definitionName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, errors.New("OpenAPI v3 JSON Schema is not ready"))
		return
	}
	util.AssembleResponse(c, cm.Data[types.OpenapiV3JSONSchema], nil)
}
