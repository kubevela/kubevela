package apiserver

import (
	"github.com/gin-gonic/gin"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/references/apiserver/util"
	"github.com/oam-dev/kubevela/references/common"
)

// AttachTrait attaches a trait to a component
func (s *APIServer) AttachTrait(c *gin.Context) {
	var body apis.TraitBody
	body.EnvName = c.Param("envName")
	body.AppName = c.Param("appName")
	body.ComponentName = c.Param("compName")

	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.InvalidArgument, "the trait attach request body is invalid")
		return
	}

	util.AssembleResponse(c, "deprecated, please use appfile to update", nil)
}

// GetTrait gets a trait by name
func (s *APIServer) GetTrait(c *gin.Context) {
	var traitType = c.Param("traitName")
	var workloadType string
	var capability types.Capability
	var err error

	if capability, err = common.GetTraitDefinition("default", s.c, &workloadType, traitType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capability, err)
}

// ListTrait lists all traits
func (s *APIServer) ListTrait(c *gin.Context) {
	var traitList []types.Capability
	var workloadName string
	var err error
	if traitList, err = common.ListTraitDefinitions("default", s.c, &workloadName); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, traitList, err)
}

// DetachTrait detaches a trait from a component
func (s *APIServer) DetachTrait(c *gin.Context) {
	util.AssembleResponse(c, "deprecated, please use appfile to update", nil)
}
