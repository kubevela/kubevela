/*
Copyright 2022 The KubeVela Authors.

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

package applicationresourcetracker

import (
	"strings"

	"github.com/oam-dev/kubevela/apis/apiextensions.core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ConvertRT2AppRT convert ResourceTracker to ApplicationResourceTracker
func ConvertRT2AppRT(rt v1beta1.ResourceTracker) v1alpha1.ApplicationResourceTracker {
	appRt := v1alpha1.ApplicationResourceTracker(rt)
	ns := types.DefaultAppNamespace
	if labels := rt.GetLabels(); labels != nil {
		ns = labels[oam.LabelAppNamespace]
	}
	appRt.SetNamespace(ns)
	appRt.SetName(strings.TrimSuffix(rt.Name, "-"+ns))
	appRt.SetGroupVersionKind(v1alpha1.ApplicationResourceTrackerGroupVersionKind)
	return appRt
}

// ConvertAppRT2RT convert ApplicationResourceTracker to ResourceTracker
func ConvertAppRT2RT(appRt v1alpha1.ApplicationResourceTracker) v1beta1.ResourceTracker {
	rt := v1beta1.ResourceTracker(appRt)
	rt.SetName(rt.Name + "-" + appRt.GetNamespace())
	rt.SetNamespace("")
	rt.SetGroupVersionKind(v1beta1.ResourceTrackerKindVersionKind)
	return rt
}
