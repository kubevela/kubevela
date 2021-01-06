package server

import (
	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/pkg/server/util"
	"github.com/oam-dev/kubevela/pkg/serverlib"
)

// GetDefinition gets OpenAPI schema from Cue section of a WorkloadDefinition/TraitDefinition
func (s *APIServer) GetDefinition(c *gin.Context) {
	definitionName := c.Param("name")
	parameter, err := serverlib.GetDefinition(definitionName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, string(parameter), nil)
}
