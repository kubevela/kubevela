package server

import (
	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
)

// UpdateWorkload updates a workload
func (s *APIServer) UpdateWorkload(c *gin.Context) {
}

// GetWorkload gets a workload by name
func (s *APIServer) GetWorkload(c *gin.Context) {
	var workloadType = c.Param("workloadName")
	var capability types.Capability
	var err error

	if capability, err = plugins.GetInstalledCapabilityWithCapName(types.TypeWorkload, workloadType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capability, err)
}

// ListWorkload lists all workloads in the cluster
func (s *APIServer) ListWorkload(c *gin.Context) {
	var workloadDefinitionList []apis.WorkloadMeta
	workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	for _, w := range workloads {
		workloadDefinitionList = append(workloadDefinitionList, apis.WorkloadMeta{
			Name:        w.Name,
			Parameters:  w.Parameters,
			Description: w.Description,
		})
	}
	util.AssembleResponse(c, workloadDefinitionList, err)
}
