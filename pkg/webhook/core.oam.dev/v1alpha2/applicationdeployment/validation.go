package applicationdeployment

import (
	"context"

	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/slice"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/webhook/common/rollout"
)

// ValidateCreate validates the ApplicationDeployment on creation
func (h *ValidatingHandler) ValidateCreate(appDeploy *v1alpha2.ApplicationDeployment) field.ErrorList {
	klog.InfoS("validate create", "name", appDeploy.Name)
	allErrs := apimachineryvalidation.ValidateObjectMeta(&appDeploy.ObjectMeta, true,
		apimachineryvalidation.NameIsDNSSubdomain, field.NewPath("metadata"))

	fldPath := field.NewPath("spec")
	target := appDeploy.Spec.TargetApplicationName
	if len(target) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("targetApplicationName"),
			"target application name cannot be empty"))
		// can't continue without target
		return allErrs
	}

	var targetApp, sourceApp v1alpha2.Application
	targetAppName := appDeploy.Spec.TargetApplicationName
	if err := h.Get(context.Background(), ktypes.NamespacedName{Namespace: appDeploy.Namespace, Name: targetAppName},
		&targetApp); err != nil {
		klog.ErrorS(err, "cannot locate target application", "target application",
			klog.KRef(appDeploy.Namespace, targetAppName))
		allErrs = append(allErrs, field.NotFound(fldPath.Child("targetApplicationName"), targetAppName))
		// can't continue without target
		return allErrs
	}
	sourceAppName := appDeploy.Spec.SourceApplicationName
	if sourceAppName != "" {
		if err := h.Get(context.Background(), ktypes.NamespacedName{Namespace: appDeploy.Namespace, Name: sourceAppName},
			&sourceApp); err != nil {
			klog.ErrorS(err, "cannot locate source application", "source application",
				klog.KRef(appDeploy.Namespace, sourceAppName))
			allErrs = append(allErrs, field.NotFound(fldPath.Child("sourceApplicationName"), sourceAppName))
		}
	}

	// validate the component spec
	allErrs = append(allErrs, validateComponent(appDeploy.Spec.ComponentList, &targetApp, &sourceApp,
		fldPath.Child("componentList"))...)

	// validate the rollout plan spec
	allErrs = append(allErrs, rollout.ValidateCreate(&appDeploy.Spec.RolloutPlan, fldPath.Child("rolloutPlan"))...)
	return allErrs
}

// validateComponent validate the ComponentList
// 1. there can only be one component or less
// 2. if there are no components, make sure the applications has only one common component so that's the default
// 3. it is contained in both source and target application
// 4. the common component has the same type
func validateComponent(componentList []string, targetApp, sourceApp *v1alpha2.Application,
	fldPath *field.Path) field.ErrorList {
	var componentErrs field.ErrorList
	var commmonComponentName string
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
		commmonComponentName = commons[0]
	} else {
		// the component need to be one of the common components
		if !slice.ContainsString(commons, componentList[0], nil) {
			klog.Error("The component does not belong to the application",
				"common components", commons, "component to upgrade", componentList[0])
			componentErrs = append(componentErrs, field.Invalid(fldPath, componentList[0],
				"it is not a common component in the application"))
			return componentErrs
		}
		commmonComponentName = componentList[0]
	}
	// check if the workload type are the same in the source and target application
	if sourceApp != nil {
		targetComp := targetApp.GetComponent(commmonComponentName)
		sourceComp := sourceApp.GetComponent(commmonComponentName)
		if targetComp.WorkloadType != sourceComp.WorkloadType {
			klog.Error("the common component have different types in the application",
				"common component", commmonComponentName, "target component type", targetComp.WorkloadType,
				"source component type", sourceComp.WorkloadType)
			componentErrs = append(componentErrs, field.Invalid(fldPath, componentList[0],
				"the common component have different types in the application"))
		}
	}
	return componentErrs
}

// ValidateUpdate validates the ApplicationDeployment on update
func (h *ValidatingHandler) ValidateUpdate(new, old *v1alpha2.ApplicationDeployment) field.ErrorList {
	klog.InfoS("validate update", "name", new.Name)
	errList := h.ValidateCreate(new)
	if len(errList) > 0 {
		return errList
	}
	fldPath := field.NewPath("spec").Child("rolloutPlan")
	return rollout.ValidateUpdate(&new.Spec.RolloutPlan, &old.Spec.RolloutPlan, fldPath)
}
