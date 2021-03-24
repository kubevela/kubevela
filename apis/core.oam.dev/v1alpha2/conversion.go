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

package v1alpha2

import (
	"fmt"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// CovertTo converts this Application to the Hub version (v1beta1).
func (app *Application) CovertTo(dst conversion.Hub) error {
	switch t := dst.(type) {
	case *v1beta1.Application:
		appv1beta1 := dst.(*v1beta1.Application)
		klog.Infof("convert *v1alpha.Application [%s] to *v1beta1.Application", app.Name)
		appv1beta1.ObjectMeta = app.ObjectMeta

		if len(app.Spec.Components) > 0 {
			componets := make([]v1beta1.ApplicationComponent, len(app.Spec.Components))
			for i, component := range app.Spec.Components {
				componets[i] = v1beta1.ApplicationComponent{
					Name:         component.Name,
					WorkloadType: component.WorkloadType,
					Settings:     component.Settings,
					Traits:       component.Traits,
					Scopes:       component.Scopes,
				}
			}
			appv1beta1.Spec.Components = componets
		}
		appv1beta1.Spec.RolloutPlan = app.Spec.RolloutPlan

		// set AppStatus
		appv1beta1.Status.RollingState = app.Status.RollingState
		appv1beta1.Status.Phase = app.Status.Phase
		appv1beta1.Status.Components = app.Status.Components
		appv1beta1.Status.Services = app.Status.Services
		appv1beta1.Status.LatestRevision = app.Status.LatestRevision

		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha2).
func (app *Application) ConvertFrom(src conversion.Hub) error {
	switch t := src.(type) {
	case *v1beta1.Application:
		appv1beta1 := src.(*v1beta1.Application)
		klog.Infof("convert *v1alpha.Application from *v1beta1.Application [%s]", appv1beta1.Name)
		app.ObjectMeta = appv1beta1.ObjectMeta

		if len(appv1beta1.Spec.Components) > 0 {
			componets := make([]ApplicationComponent, len(appv1beta1.Spec.Components))
			for i, component := range appv1beta1.Spec.Components {
				componets[i] = ApplicationComponent{
					Name:         component.Name,
					WorkloadType: component.WorkloadType,
					Settings:     component.Settings,
					Traits:       component.Traits,
					Scopes:       component.Scopes,
				}
			}
			app.Spec.Components = componets
		}
		app.Spec.RolloutPlan = appv1beta1.Spec.RolloutPlan

		// set AppStatus
		app.Status.RollingState = appv1beta1.Status.RollingState
		app.Status.Phase = appv1beta1.Status.Phase
		app.Status.Components = appv1beta1.Status.Components
		app.Status.Services = appv1beta1.Status.Services
		app.Status.LatestRevision = appv1beta1.Status.LatestRevision

		return nil
	default:
		return fmt.Errorf("unsupported type %v", t)
	}
}
