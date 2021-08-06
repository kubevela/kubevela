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

package services

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/proto/model"
)

// CapabilityService capability service
type CapabilityService struct {
	k8sClient client.Client
}

// NewCapabilityService create capability service
func NewCapabilityService(client client.Client) *CapabilityService {

	return &CapabilityService{
		k8sClient: client,
	}
}

// ListCapabilities list method for capability configmap
func (s *CapabilityService) ListCapabilities(c echo.Context) error {
	var cmList v1.ConfigMapList
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"capability": "configdata",
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labels)
	if err != nil {
		return err
	}
	err = s.k8sClient.List(context.Background(), &cmList, &client.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return err
	}

	var capabilityList = make([]*model.Capability, 0, len(cmList.Items))
	for i, c := range cmList.Items {
		UpdateInt, err := strconv.ParseInt(cmList.Items[i].Data["UpdatedAt"], 10, 64)
		if err != nil {
			return err
		}
		capability := model.Capability{
			Name:        c.Name,
			UpdatedAt:   UpdateInt,
			Desc:        cmList.Items[i].Data["Desc"],
			Type:        cmList.Items[i].Data["Type"],
			CatalogName: cmList.Items[i].Data["CatalogName"],
			JsonSchema:  cmList.Items[i].Data["initializer"],
		}
		capabilityList = append(capabilityList, &capability)
	}
	return c.JSON(http.StatusOK, model.CapabilityListResponse{Capabilities: capabilityList})
}

// GetCapability get method for capability configmap
func (s *CapabilityService) GetCapability(c echo.Context) error {
	capabilityName := c.Param("capabilityName")

	var cm v1.ConfigMap
	err := s.k8sClient.Get(context.Background(), client.ObjectKey{Namespace: DefaultVelaNamespace, Name: capabilityName}, &cm)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, fmt.Sprintf("get config for %s failed %s", capabilityName, err.Error()))
	}
	var capability = model.Capability{
		Name:        capabilityName,
		CatalogName: capabilityName,
		JsonSchema:  cm.Data["initializer"],
	}

	return c.JSON(http.StatusOK, model.CapabilityResponse{Capability: &capability})
}

// InstallCapability installs a capability into a cluster
//
// TODO: implement this method,
// install logic is same as the `vela` cli, we should find a way to reuse these code:
// https://github.com/oam-dev/kubevela/blob/9a10e967eec8e42a8aa284ddb20fde204696aa69/references/common/capability.go#L88
func (s *CapabilityService) InstallCapability(c echo.Context) error {
	capabilityName := c.Param("capabilityName")
	clusterName := c.QueryParam("clusterName")

	log.Logger.Debugf("installing capability %s to cluster %s", capabilityName, clusterName)

	return c.JSON(http.StatusOK, true)
}
