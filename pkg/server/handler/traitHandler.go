package handler

import (
	"strconv"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/server/util"
	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Trait related handlers
func AttachTrait(c *gin.Context) {
	var body apis.TraitBody
	body.EnvName = c.Param("envName")
	body.WorkloadName = c.Param("appName")
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.InvalidArgument, "the trait attach request body is invalid")
		return
	}
	ctrl.Log.Info("request parameters body:", "body", body)
	msg, err := oam.AttachTrait(c, body)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, nil)
}

func GetTrait(c *gin.Context) {
	var traitType = c.Param("traitName")
	var workloadType string
	var capability types.Capability
	var err error

	if capability, err = oam.GetTraitDefinition(&workloadType, traitType); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, capability, err)
}

func ListTrait(c *gin.Context) {
	var traitList []types.Capability
	var workloadName string
	var err error
	if traitList, err = oam.ListTraitDefinitions(&workloadName); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err)
		return
	}
	util.AssembleResponse(c, traitList, err)
}

func DetachTrait(c *gin.Context) {
	envName := c.Param("envName")
	traitType := c.Param("traitName")
	workloadName := c.Param("appName")
	var staging = false
	var err error
	if stagingStr := c.Param("staging"); stagingStr != "" {
		if staging, err = strconv.ParseBool(stagingStr); err != nil {
			util.HandleError(c, util.StatusInternalServerError, err.Error())
			return
		}
	}
	msg, err := oam.DetachTrait(c, envName, traitType, workloadName, "", staging)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, nil)
}
