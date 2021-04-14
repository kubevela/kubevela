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
	"github.com/oam-dev/kubevela/references/apiserver/util"
	"github.com/oam-dev/kubevela/references/common"
	"github.com/oam-dev/kubevela/references/plugins"

	"github.com/gin-gonic/gin"
)

// AddCapabilityCenter adds and synchronizes a capability center from remote
func (s *APIServer) AddCapabilityCenter(c *gin.Context) {
	var body plugins.CapCenterConfig
	if err := c.ShouldBindJSON(&body); err != nil {
		util.HandleError(c, util.StatusInternalServerError, "the add capability center request body is invalid")
		return
	}
	if err := common.AddCapabilityCenter(body.Name, body.Address, body.Token); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, "Successfully configured capability center and synchronized from remote", nil)
}

// ListCapabilityCenters list all added capability centers
func (s *APIServer) ListCapabilityCenters(c *gin.Context) {
	capabilityCenterList, err := common.ListCapabilityCenters()
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, capabilityCenterList, nil)
}

// SyncCapabilityCenter synchronizes capability center from remote
func (s *APIServer) SyncCapabilityCenter(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityCenterName")
	if err := common.SyncCapabilityCenter(capabilityCenterName); err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, "sync finished", nil)
}

// AddCapabilityIntoCluster adds specific capability into cluster
func (s *APIServer) AddCapabilityIntoCluster(c *gin.Context) {
	cap := c.Param("capabilityCenterName") + "/" + c.Param("capabilityName")
	msg, err := common.AddCapabilityIntoCluster(s.KubeClient, s.dm, cap)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError)
		return
	}
	util.AssembleResponse(c, msg, nil)
}

// DeleteCapabilityCenter deltes a capability cernter already added
func (s *APIServer) DeleteCapabilityCenter(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityCenterName")
	msg, err := common.RemoveCapabilityCenter(capabilityCenterName)
	util.AssembleResponse(c, msg, err)
}

// RemoveCapabilityFromCluster remove a specific capability from cluster
func (s *APIServer) RemoveCapabilityFromCluster(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityName")
	// TODO get namespace from env
	msg, err := common.RemoveCapabilityFromCluster("default", s.c, s.KubeClient, capabilityCenterName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, msg, nil)
}

// ListCapabilities lists capabilities of a capability center
func (s *APIServer) ListCapabilities(c *gin.Context) {
	capabilityCenterName := c.Param("capabilityName")
	capabilityList, err := common.ListCapabilities("default", s.c, capabilityCenterName)
	if err != nil {
		util.HandleError(c, util.StatusInternalServerError, err.Error())
		return
	}
	util.AssembleResponse(c, capabilityList, nil)
}
