/*
 Copyright 2021. The KubeVela Authors.

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

package componentdefinition

import (
	"context"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

type handler struct {
	client.Client
	dm discoverymapper.DiscoveryMapper
	cd *v1beta1.ComponentDefinition
}

func (h *handler) CreateWorkloadDefinition(ctx context.Context) (util.WorkloadType, error) {
	var workloadType = util.ComponentDef
	var workloadName = h.cd.Name
	if h.cd.Spec.Workload.Type != "" {
		workloadType = util.ReferWorkload
		workloadName = h.cd.Spec.Workload.Type
	}
	if h.cd.Spec.Schematic != nil && h.cd.Spec.Schematic.HELM != nil {
		workloadType = util.HELMDef
	}
	if h.cd.Spec.Schematic != nil && h.cd.Spec.Schematic.KUBE != nil {
		workloadType = util.KubeDef
	}

	wd := new(v1beta1.WorkloadDefinition)
	err := h.Get(ctx, client.ObjectKey{Namespace: h.cd.Namespace, Name: workloadName}, wd)
	if err != nil {
		switch workloadType {
		case util.ReferWorkload:
			klog.Infof("ComponentDefinition %s refer to wrong Workload", h.cd.Name)
			return workloadType, err
		default:
			if !kerrors.IsNotFound(err) {
				return workloadType, err
			}
			newCd := h.cd.DeepCopy()
			if err := util.ConvertComponentDef2WorkloadDef(h.dm, newCd, wd); err != nil {
				return workloadType, fmt.Errorf("convert WorkloadDefinition %s error %w", h.cd.Name, err)
			}
			owners := []metav1.OwnerReference{{
				APIVersion: v1beta1.SchemeGroupVersion.String(),
				Kind:       v1beta1.ComponentDefinitionKind,
				Name:       h.cd.Name,
				UID:        h.cd.UID,
				Controller: pointer.BoolPtr(true),
			}}
			wd.SetOwnerReferences(owners)
			if err := h.Create(ctx, wd); err != nil {
				return workloadType, fmt.Errorf("create converted WorkloadDefinition %s error %w", h.cd.Name, err)
			}
		}
	}
	return workloadType, nil
}
