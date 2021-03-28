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

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/references/apiserver/util"
	"github.com/oam-dev/kubevela/references/plugins"
)

// UpdateWorkload updates a workload
func (s *APIServer) UpdateWorkload(c *gin.Context) {
}

// GetWorkload gets a workload by name
func (s *APIServer) GetWorkload(c *gin.Context) {
	var workloadType = c.Param("componentName")
	var capability types.Capability
	var err error

	if capability, err = plugins.GetInstalledCapabilityWithCapName(types.TypeComponentDefinition, workloadType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capability, err)
}

// ListWorkload lists all workloads in the cluster
func (s *APIServer) ListWorkload(c *gin.Context) {
	var componentDefinitionList []apis.WorkloadMeta
	workloads, err := plugins.LoadInstalledCapabilityWithType("default", s.c, types.TypeComponentDefinition)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	for _, w := range workloads {
		componentDefinitionList = append(componentDefinitionList, apis.WorkloadMeta{
			Name:        w.Name,
			Parameters:  w.Parameters,
			Description: w.Description,
		})
	}
	util.AssembleResponse(c, componentDefinitionList, err)
}
