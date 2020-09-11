package handler

import (
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/util"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AddCapabilityCenter(c *gin.Context) {
	var body plugins.CapCenterConfig
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.StatusInternalServerError, "the add capability center request body is invalid")
		return
	}
	if err := oam.AddCapabilityCenter(body.Name, body.Address, body.Token); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	if err := oam.SyncCapabilityFromCenter(body.Name, body.Address, body.Token); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, "Successfully configured capability center and synchronized from remote", nil)
}

func ListCapabilityCenters(c *gin.Context) {
	capabilityCenterList, err := oam.ListCapabilityCenters()
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, capabilityCenterList, nil)
}

func SyncCapabilityCenter(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityCenterName")
	if err := oam.SyncCapabilityCenter(capabilityCenterName); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, "sync finished", nil)
}

func AddCapabilityIntoCluster(c *gin.Context) {
	cap := c.Param("capabilityCenterName") + "/" + c.Param("capabilityName")
	kubeClient := c.MustGet("KubeClient")
	msg, err := oam.AddCapabilityIntoCluster(kubeClient.(client.Client), cap)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError)
		return
	}
	util.AssembleResponse(c, msg, nil)
}

func DeleteCapabilityCenter(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityCenterName")
	msg, err := oam.RemoveCapabilityCenter(capabilityCenterName)
	util.AssembleResponse(c, msg, err)
}

func RemoveCapabilityFromCluster(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityName")
	kubeClient := c.MustGet("KubeClient")
	msg, err := oam.RemoveCapabilityFromCluster(kubeClient.(client.Client), capabilityCenterName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, nil)
}

func ListCapabilities(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityName")
	capabilityList, err := oam.ListCapabilities(capabilityCenterName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, capabilityList, nil)
}
