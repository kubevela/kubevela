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

package v1alpha2

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ApplicationV1alpha2ToV1beta1 will convert v1alpha2 to v1beta1
func ApplicationV1alpha2ToV1beta1(v1a2 *Application, v1b1 *v1beta1.Application) {
	// 1) convert metav1.TypeMeta
	// apiVersion and Kind automatically converted

	// 2) convert metav1.ObjectMeta
	v1b1.ObjectMeta = *v1a2.ObjectMeta.DeepCopy()

	// 3) convert Spec  ApplicationSpec
	// 3.1) convert Spec.Components
	for _, comp := range v1a2.Spec.Components {

		// convert trait, especially for `.name` -> `.type`
		var traits = make([]common.ApplicationTrait, len(comp.Traits))
		for j, trait := range comp.Traits {
			traits[j] = common.ApplicationTrait{
				Type:       trait.Name,
				Properties: trait.Properties.DeepCopy(),
			}
		}

		// deep copy scopes
		scopes := make(map[string]string)
		for k, v := range comp.Scopes {
			scopes[k] = v
		}
		// convert component
		// `.settings` -> `.properties`
		v1b1.Spec.Components = append(v1b1.Spec.Components, common.ApplicationComponent{
			Name:       comp.Name,
			Type:       comp.WorkloadType,
			Properties: comp.Settings.DeepCopy(),
			Traits:     traits,
			Scopes:     scopes,
		})
	}

	// 4) convert Status common.AppStatus
	v1b1.Status = *v1a2.Status.DeepCopy()
}

// ConvertTo converts this Application to the Hub version (v1beta1 only for now).
func (app *Application) ConvertTo(dst conversion.Hub) error {
	switch convertedApp := dst.(type) {
	case *v1beta1.Application:
		klog.Infof("convert *v1alpha2.Application [%s] to *v1beta1.Application", app.Name)
		ApplicationV1alpha2ToV1beta1(app, convertedApp)
		return nil
	default:
	}
	return fmt.Errorf("unsupported convertTo object %v", reflect.TypeOf(dst))
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha2).
func (app *Application) ConvertFrom(src conversion.Hub) error {
	switch sourceApp := src.(type) {
	case *v1beta1.Application:

		klog.Infof("convert *v1alpha2.Application from *v1beta1.Application [%s]", sourceApp.Name)

		// 1) convert metav1.TypeMeta
		// apiVersion and Kind automatically converted

		// 2) convert metav1.ObjectMeta
		app.ObjectMeta = *sourceApp.ObjectMeta.DeepCopy()

		// 3) convert Spec  ApplicationSpec
		// 3.1) convert Spec.Components
		for _, comp := range sourceApp.Spec.Components {

			// convert trait, especially for `.type` -> `.name`
			var traits = make([]ApplicationTrait, len(comp.Traits))
			for j, trait := range comp.Traits {
				traits[j] = ApplicationTrait{
					Name:       trait.Type,
					Properties: trait.Properties.DeepCopy(),
				}
			}

			// deep copy scopes
			scopes := make(map[string]string)
			for k, v := range comp.Scopes {
				scopes[k] = v
			}
			// convert component
			//  `.properties` -> `.settings`

			var compProperties runtime.RawExtension

			if comp.Properties != nil {
				compProperties = *comp.Properties.DeepCopy()
			}

			app.Spec.Components = append(app.Spec.Components, ApplicationComponent{
				Name:         comp.Name,
				WorkloadType: comp.Type,
				Settings:     compProperties,
				Traits:       traits,
				Scopes:       scopes,
			})
		}

		// 4) convert Status common.AppStatus
		app.Status = *sourceApp.Status.DeepCopy()
		return nil
	default:
	}
	return fmt.Errorf("unsupported ConvertFrom object %v", reflect.TypeOf(src))
}
