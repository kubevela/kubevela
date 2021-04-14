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

package applicationrollout

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/slice"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/webhook/common/rollout"
)

// ValidateCreate validates the AppRollout on creation
func (h *ValidatingHandler) ValidateCreate(appRollout *v1beta1.AppRollout) field.ErrorList {
	klog.InfoS("validate create", "name", appRollout.Name)
	allErrs := apimachineryvalidation.ValidateObjectMeta(&appRollout.ObjectMeta, true,
		apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))

	fldPath := field.NewPath("spec")
	target := appRollout.Spec.TargetAppRevisionName
	if len(target) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("targetApplicationName"),
			"target application name cannot be empty"))
		// can't continue without target
		return allErrs
	}

	if appRollout.DeletionTimestamp.IsZero() {
		var targetAppRevision v1beta1.ApplicationRevision
		sourceAppRevision := &v1beta1.ApplicationRevision{}
		targetAppName := appRollout.Spec.TargetAppRevisionName
		if err := h.Get(context.Background(), ktypes.NamespacedName{Namespace: appRollout.Namespace, Name: targetAppName},
			&targetAppRevision); err != nil {
			klog.ErrorS(err, "cannot locate target application revision", "target application revision",
				klog.KRef(appRollout.Namespace, targetAppName))
			allErrs = append(allErrs, field.NotFound(fldPath.Child("targetAppRevisionName"), targetAppName))
			// can't continue without target
			return allErrs
		}
		sourceAppName := appRollout.Spec.SourceAppRevisionName
		if sourceAppName != "" {
			if err := h.Get(context.Background(), ktypes.NamespacedName{Namespace: appRollout.Namespace, Name: sourceAppName},
				sourceAppRevision); err != nil {
				klog.ErrorS(err, "cannot locate source application revision", "source application revision",
					klog.KRef(appRollout.Namespace, sourceAppName))
				allErrs = append(allErrs, field.NotFound(fldPath.Child("sourceAppRevisionName"), sourceAppName))
			}
		} else {
			sourceAppRevision = nil
		}
		var sourceApp *v1alpha2.ApplicationConfiguration
		targetApp, err := oamutil.RawExtension2AppConfig(targetAppRevision.Spec.ApplicationConfiguration)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("TargetAppRevisionName"), targetAppName,
				fmt.Sprintf("the targeted app revision is corrupted,  err = `%s`", err)))
		}
		if sourceAppRevision != nil {
			sourceApp, err = oamutil.RawExtension2AppConfig(sourceAppRevision.Spec.ApplicationConfiguration)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("SourceAppRevisionName"), sourceAppName,
					fmt.Sprintf("the source app revision is corrupted,  err = `%s`", err)))
			}
		}
		// validate the component spec
		allErrs = append(allErrs, validateComponent(appRollout.Spec.ComponentList, targetApp, sourceApp,
			fldPath.Child("componentList"))...)
	}

	// validate the rollout plan spec
	allErrs = append(allErrs, rollout.ValidateCreate(h, &appRollout.Spec.RolloutPlan, fldPath.Child("rolloutPlan"))...)
	return allErrs
}

// validateComponent validate the ComponentList
// 1. there can only be one component or less
// 2. if there are no components, make sure the applications has only one common component so that's the default
// 3. it is contained in both source and target application
// 4. the common component has the same type
func validateComponent(componentList []string, targetApp, sourceApp *v1alpha2.ApplicationConfiguration,
	fldPath *field.Path) field.ErrorList {
	var componentErrs field.ErrorList
	var commonComponentName string
	if len(componentList) > 1 {
		componentErrs = append(componentErrs, field.TooLong(fldPath, componentList, 1))
		return componentErrs
	}
	commons := FindCommonComponent(targetApp, sourceApp)
	if len(componentList) == 0 {
		// we need to find the default
		if len(commons) != 1 {
			// we cannot find a default component if there are multiple
			klog.Error("there are more than one common component", "common component", commons)
			componentErrs = append(componentErrs, field.TooMany(fldPath, len(commons), 1))
			return componentErrs
		}
		commonComponentName = commons[0]
	} else {
		// the component need to be one of the common components
		if !slice.ContainsString(commons, componentList[0], nil) {
			klog.Error("The component does not belong to the application",
				"common components", commons, "component to upgrade", componentList[0])
			componentErrs = append(componentErrs, field.Invalid(fldPath, componentList[0],
				"it is not a common component in the application"))
			return componentErrs
		}
		commonComponentName = componentList[0]
	}
	// check if the workload type are the same in the source and target application
	if len(commonComponentName) == 0 {
		klog.Error("the common component have different types in the application",
			"common component", commonComponentName)
		componentErrs = append(componentErrs, field.Invalid(fldPath, componentList[0],
			"the common component have different types in the application"))
	}

	return componentErrs
}

// ValidateUpdate validates the AppRollout on update
func (h *ValidatingHandler) ValidateUpdate(new, old *v1beta1.AppRollout) field.ErrorList {
	klog.InfoS("validate update", "name", new.Name)
	errList := h.ValidateCreate(new)
	fldPath := field.NewPath("spec").Child("rolloutPlan")

	if len(errList) > 0 {
		return errList
	}
	// we can only reuse the rollout after reaching terminating state if the target and source has changed
	if old.Status.RollingState == v1alpha1.RolloutSucceedState ||
		old.Status.RollingState == v1alpha1.RolloutFailedState {
		if old.Spec.SourceAppRevisionName == new.Spec.SourceAppRevisionName &&
			old.Spec.TargetAppRevisionName == new.Spec.TargetAppRevisionName {
			if !apiequality.Semantic.DeepEqual(&old.Spec.RolloutPlan, &new.Spec.RolloutPlan) {
				errList = append(errList, field.Invalid(fldPath, new.Spec,
					"a successful or failed rollout cannot be modified without changing the target or the source"))
				return errList
			}
		}
	}

	return rollout.ValidateUpdate(h, &new.Spec.RolloutPlan, &old.Spec.RolloutPlan, fldPath)
}
