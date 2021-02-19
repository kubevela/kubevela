package applicationdeployment

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	appUtil "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationdeployment"
)

// extractWorkloadTypeAndGVK extracts the workload type and gvk
func (r *Reconciler) extractWorkloadTypeAndGVK(ctx context.Context, componentList []string, targetApp,
	sourceApp *corev1alpha2.Application) (string, *schema.GroupVersionKind, error) {
	var componentType string
	// assume that the validator webhook has already guaranteed that there is no more than one component for now
	if len(componentList) == 0 {
		// we need to find a default component
		commons := appUtil.FindCommonComponent(targetApp, sourceApp)
		if len(commons) != 1 {
			return "", nil, fmt.Errorf("cannot find a default component, too many common components: %+v", commons)
		}
		componentType = commons[0]
	} else {
		componentType = componentList[0]
	}
	// get the workload definition
	// the validator webhook has checked that source and the target are the same type
	wd := new(corev1alpha2.WorkloadDefinition)
	err := oamutil.GetDefinition(ctx, r, wd, componentType)
	if err != nil {
		return "", nil, errors.Wrap(err, fmt.Sprintf("failed to get workload definition %s", componentType))
	}
	// get the CR kind from the definitionRef
	gvk, err := oamutil.GetGVKFromDefinition(r.dm, wd.Spec.Reference)
	if err != nil {
		return "", nil, errors.Wrap(err, fmt.Sprintf("failed to get workload GVK from definition ref %s",
			wd.Spec.Reference))
	}
	return componentType, &gvk, nil
}

// fetchWorkload based on the component type and the application and its gvk
func (r *Reconciler) fetchWorkloads(ctx context.Context, targetApp, sourceApp *corev1alpha2.Application, workloadType string,
	workloadGVK *schema.GroupVersionKind) (*unstructured.Unstructured, *unstructured.Unstructured, error) {
	targetWorkload, err := r.fetchWorkload(ctx, targetApp, workloadType, *workloadGVK)
	if err != nil {
		return nil, nil, err
	}
	klog.InfoS("get the target workload we need to work on", "targetWorkload", klog.KObj(targetWorkload))

	if sourceApp == nil {
		return targetWorkload, nil, nil
	}
	sourceWorkload, err := r.fetchWorkload(ctx, sourceApp, workloadType, *workloadGVK)
	if err != nil {
		return nil, nil, err
	}
	if sourceWorkload != nil {
		klog.InfoS("get the source workload we need to work on", "sourceWorkload", klog.KObj(sourceWorkload))
	}

	return targetWorkload, sourceWorkload, nil
}

// fetchWorkload based on the component type and the application and its gvk
func (r *Reconciler) fetchWorkload(ctx context.Context, app *corev1alpha2.Application, componentType string,
	gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	// get the component definition in the app,
	comp := app.GetComponent(componentType)
	if comp == nil {
		return nil, fmt.Errorf("cannot find the component %s in the application", componentType)
	}
	// get the workload given GVK and name
	workload, err := oamutil.GetObjectGivenGVKAndName(ctx, r, gvk, app.GetNamespace(), comp.Name)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get workload %s with gvk %+v ", componentType, gvk))
	}
	return workload, nil
}
