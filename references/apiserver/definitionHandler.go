package apiserver

import (
	"github.com/gin-gonic/gin"

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
	parameter, err := common.GetDefinition(definitionName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, string(parameter), nil)
}
