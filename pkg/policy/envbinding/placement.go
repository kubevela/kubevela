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

package envbinding

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ReadPlacementDecisions read placement decisions from application status, return (decisions, if decision is made, error)
// Deprecated As it is only used in EnvBinding policy
func ReadPlacementDecisions(app *v1beta1.Application, policyName string, envName string) ([]v1alpha1.PlacementDecision, bool, error) {
	envBindingStatus, err := GetEnvBindingPolicyStatus(app, policyName)
	if err != nil || envBindingStatus == nil {
		return nil, false, err
	}
	for _, envStatus := range envBindingStatus.Envs {
		if envStatus.Env == envName {
			return envStatus.Placements, true, nil
		}
	}
	return nil, false, nil
}

// updateClusterConnections update cluster connection in envbinding status with decisions
func updateClusterConnections(status *v1alpha1.EnvBindingStatus, decisions []v1alpha1.PlacementDecision, app *v1beta1.Application) {
	var currentRev string
	if app.Status.LatestRevision != nil {
		currentRev = app.Status.LatestRevision.Name
	}
	clusterMap := map[string]bool{}
	for _, decision := range decisions {
		clusterMap[decision.Cluster] = true
	}
	for clusterName := range clusterMap {
		exists := false
		for idx, conn := range status.ClusterConnections {
			if conn.ClusterName == clusterName {
				exists = true
				status.ClusterConnections[idx].LastActiveRevision = currentRev
				break
			}
		}
		if !exists {
			status.ClusterConnections = append(status.ClusterConnections, v1alpha1.ClusterConnection{
				ClusterName:        clusterName,
				LastActiveRevision: currentRev,
			})
		}
	}
}

// WritePlacementDecisions write placement decisions into application status
// Deprecated As it is only used in EnvBinding policy
func WritePlacementDecisions(app *v1beta1.Application, policyName string, envName string, decisions []v1alpha1.PlacementDecision) error {
	statusExists := false
	for idx, policyStatus := range app.Status.PolicyStatus {
		if policyStatus.Name == policyName && policyStatus.Type == v1alpha1.EnvBindingPolicyType {
			envBindingStatus := &v1alpha1.EnvBindingStatus{}
			err := json.Unmarshal(policyStatus.Status.Raw, envBindingStatus)
			if err != nil {
				return err
			}
			insert := true
			for _idx, envStatus := range envBindingStatus.Envs {
				if envStatus.Env == envName {
					// TODO gc
					envBindingStatus.Envs[_idx].Placements = decisions
					insert = false
					break
				}
			}
			if insert {
				envBindingStatus.Envs = append(envBindingStatus.Envs, v1alpha1.EnvStatus{
					Env:        envName,
					Placements: decisions,
				})
			}
			updateClusterConnections(envBindingStatus, decisions, app)
			bs, err := json.Marshal(envBindingStatus)
			if err != nil {
				return err
			}
			app.Status.PolicyStatus[idx].Status = &runtime.RawExtension{Raw: bs}
			statusExists = true
			break
		}
	}
	if !statusExists {
		envBindingStatus := &v1alpha1.EnvBindingStatus{
			Envs: []v1alpha1.EnvStatus{{
				Env:        envName,
				Placements: decisions,
			}},
		}
		updateClusterConnections(envBindingStatus, decisions, app)
		bs, err := json.Marshal(envBindingStatus)
		if err != nil {
			return err
		}
		app.Status.PolicyStatus = append(app.Status.PolicyStatus, common.PolicyStatus{
			Name:   policyName,
			Type:   v1alpha1.EnvBindingPolicyType,
			Status: &runtime.RawExtension{Raw: bs},
		})
	}
	return nil
}
