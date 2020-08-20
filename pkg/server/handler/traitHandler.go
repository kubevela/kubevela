package handler

import (
	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/oam"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
	"github.com/gin-gonic/gin"
)

// Trait related handlers
func CreateTrait(c *gin.Context) {
}

func UpdateTrait(c *gin.Context) {
}

func GetTrait(c *gin.Context) {
	var traitType = c.Param("traitName")
	var workloadType string
	var capabilityList []types.Capability
	var err error

	if capabilityList, err = oam.ListTraitDefinitions(&workloadType, traitType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	if len(capabilityList) == 0 {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capabilityList[0], err)
}

func ListTrait(c *gin.Context) {
	var traitList []types.Capability
	var workloadName string
	var err error
	if traitList, err = oam.ListTraitDefinitions(&workloadName, ""); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, traitList, err)
}

func DeleteTrait(c *gin.Context) {
}
