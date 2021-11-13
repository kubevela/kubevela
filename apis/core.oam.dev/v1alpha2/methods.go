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

// This code is manually implemented, but should be generated in the future.

package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// GetCondition of this ApplicationConfiguration.
func (ac *ApplicationConfiguration) GetCondition(ct condition.ConditionType) condition.Condition {
	return ac.Status.GetCondition(ct)
}

// SetConditions of this ApplicationConfiguration.
func (ac *ApplicationConfiguration) SetConditions(c ...condition.Condition) {
	ac.Status.SetConditions(c...)
}

// GetCondition of this Component.
func (cm *Component) GetCondition(ct condition.ConditionType) condition.Condition {
	return cm.Status.GetCondition(ct)
}

// SetConditions of this Component.
func (cm *Component) SetConditions(c ...condition.Condition) {
	cm.Status.SetConditions(c...)
}

// GetCondition of this HealthScope.
func (hs *HealthScope) GetCondition(ct condition.ConditionType) condition.Condition {
	return hs.Status.GetCondition(ct)
}

// SetConditions of this HealthScope.
func (hs *HealthScope) SetConditions(c ...condition.Condition) {
	hs.Status.SetConditions(c...)
}

// GetWorkloadReferences to get all workload references for scope.
func (hs *HealthScope) GetWorkloadReferences() []corev1.ObjectReference {
	return hs.Spec.WorkloadReferences
}

// AddWorkloadReference to add a workload reference to this scope.
func (hs *HealthScope) AddWorkloadReference(r corev1.ObjectReference) {
	hs.Spec.WorkloadReferences = append(hs.Spec.WorkloadReferences, r)
}
