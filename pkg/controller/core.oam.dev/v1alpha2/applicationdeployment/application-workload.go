package applicationdeployment

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	appUtil "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationdeployment"
)

// extractWorkloads extracts the workloads from the source and target applicationConfig
func (r *Reconciler) extractWorkloads(ctx context.Context, componentList []string, targetApp,
	sourceApp *corev1alpha2.ApplicationConfiguration) (*unstructured.Unstructured, *unstructured.Unstructured, error) {
	var componentName string
	if len(componentList) == 0 {
		// we need to find a default component
		commons := appUtil.FindCommonComponent(targetApp, sourceApp)
		if len(commons) != 1 {
			return nil, nil, fmt.Errorf("cannot find a default component, too many common components: %+v", commons)
		}
		componentName = commons[0]
	} else {
		// assume that the validator webhook has already guaranteed that there is no more than one component for now
		// and the component exists in both the target and source app
		componentName = componentList[0]
	}
	// get the workload definition
	// the validator webhook has checked that source and the target are the same type
	targetWorkload, err := r.fetchWorkload(ctx, componentName, targetApp)
	if err != nil {
		return nil, nil, err
	}
	klog.InfoS("successfully get the target workload we need to work on", "targetWorkload", klog.KObj(targetWorkload))
	if sourceApp != nil {
		sourceWorkload, err := r.fetchWorkload(ctx, componentName, sourceApp)
		if err != nil {
			return nil, nil, err
		}
		klog.InfoS("successfully get the source workload we need to work on", "sourceWorkload",
			klog.KObj(sourceWorkload))
		return targetWorkload, sourceWorkload, nil
	}
	return targetWorkload, nil, nil
}

// fetchWorkload based on the component and the appConfig
func (r *Reconciler) fetchWorkload(ctx context.Context, componentName string,
	targetApp *corev1alpha2.ApplicationConfiguration) (*unstructured.Unstructured, error) {
	var targetAcc *corev1alpha2.ApplicationConfigurationComponent
	for _, acc := range targetApp.Spec.Components {
		if utils.ExtractComponentName(acc.RevisionName) == componentName {
			targetAcc = acc.DeepCopy()
		}
	}
	// can't happen as we just searched the appConfig
	if targetAcc == nil {
		klog.Error("The component does not belong to the application",
			"components", targetApp.Spec.Components, "component to upgrade", componentName)
		return nil, fmt.Errorf("the component %s does not belong to the application with components %+v",
			componentName, targetApp.Spec.Components)
	}
	revision, err := utils.ExtractRevision(targetAcc.RevisionName)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get revision given revision name %s",
			targetAcc.RevisionName))
	}

	// get the component given the component revision
	component, _, err := oamutil.GetComponent(ctx, r, *targetAcc, targetApp.GetNamespace())
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get component given its revision %s",
			targetAcc.RevisionName))
	}
	// get the workload template in the component
	w, err := oamutil.RawExtension2Unstructured(&component.Spec.Workload)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get component given revision %s", targetAcc.RevisionName))
	}
	// reuse the same appConfig controller logic that determines the workload name given an ACC
	applicationconfiguration.SetAppWorkloadInstanceName(componentName, w, revision)
	// get the real workload object from api-server given GVK and name
	workload, err := oamutil.GetObjectGivenGVKAndName(ctx, r, w.GroupVersionKind(), targetApp.GetNamespace(), w.GetName())
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get workload %s with gvk %+v ", w.GetName(), w.GroupVersionKind()))
	}

	return workload, nil
}
