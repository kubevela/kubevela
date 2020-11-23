//nolint:golint
package server

import (
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/server/util"

	"github.com/gin-gonic/gin"
)

func (s *APIServer) AddCapabilityCenter(c *gin.Context) {
	var body plugins.CapCenterConfig
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.StatusInternalServerError, "the add capability center request body is invalid")
		return
	}
	if err := oam.AddCapabilityCenter(body.Name, body.Address, body.Token); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, "Successfully configured capability center and synchronized from remote", nil)
}

func (s *APIServer) ListCapabilityCenters(c *gin.Context) {
	capabilityCenterList, err := oam.ListCapabilityCenters()
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, capabilityCenterList, nil)
}

func (s *APIServer) SyncCapabilityCenter(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityCenterName")
	if err := oam.SyncCapabilityCenter(capabilityCenterName); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, "sync finished", nil)
}

func (s *APIServer) AddCapabilityIntoCluster(c *gin.Context) {
	cap := c.Param("capabilityCenterName") + "/" + c.Param("capabilityName")
	msg, err := oam.AddCapabilityIntoCluster(s.KubeClient, s.dm, cap)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError)
		return
	}
	util.AssembleResponse(c, msg, nil)
}

func (s *APIServer) DeleteCapabilityCenter(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityCenterName")
	msg, err := oam.RemoveCapabilityCenter(capabilityCenterName)
	util.AssembleResponse(c, msg, err)
}

func (s *APIServer) RemoveCapabilityFromCluster(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityName")
	msg, err := oam.RemoveCapabilityFromCluster(s.KubeClient, capabilityCenterName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, nil)
}

func (s *APIServer) ListCapabilities(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityName")
	capabilityList, err := oam.ListCapabilities(capabilityCenterName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, capabilityList, nil)
}
