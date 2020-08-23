package handler

import (
	"github.com/cloud-native-application/rudrx/pkg/oam"
	"github.com/cloud-native-application/rudrx/pkg/plugins"
	"github.com/cloud-native-application/rudrx/pkg/server/apis"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
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

func InstallCapabilityIntoCluster(c *gin.Context) {
	var body apis.CapabilityMeta
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.StatusInternalServerError, "the install capability into cluster request body is invalid")
		return
	}
	cap := body.CapabilityCenterName + "/" + body.CapabilityName
	kubeClient := c.MustGet("KubeClient")
	msg, err := oam.AddCapabilityIntoCluster(cap, kubeClient.(client.Client))
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError)
		return
	}
	util.AssembleResponse(c, msg, nil)
}
