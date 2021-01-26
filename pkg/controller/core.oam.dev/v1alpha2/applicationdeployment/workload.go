package applicationdeployment

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	appUtil "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationdeployment"
)

// extractWorkload extracts the workload
func (r *Reconciler) extractWorkload(componentList []string, targetApp,
	sourceApp *corev1alpha2.Application) (*unstructured.Unstructured, *unstructured.Unstructured, error) {
	var componentType string
	// assume that the validator webhook has already guaranteed that there is no more than one component for now
	if len(componentList) == 0 {
		// we need to find a default component
		commons := appUtil.FindCommonComponent(targetApp, sourceApp)
		if len(commons) != 1 {
			return nil, nil, fmt.Errorf("cannot find a default component, too many common components: %+v", commons)
		}
		componentType = commons[0]
	} else {
		componentType = componentList[0]
	}
	// get the workload definition
	// the validator webhook has checked that source and the target are the same type
	wd, err := oamutil.GetWorkloadDefinition(r, componentType)
	if err != nil {
		return nil, nil, errors.Wrap(err, fmt.Sprintf("failed to get workload definition %s", componentType))
	}
	// get the CR kind from the definitionRef
	gvk, err := oamutil.GetGVKFromDefinition(r.dm, wd.Spec.Reference)
	if err != nil {
		return nil, nil, errors.Wrap(err, fmt.Sprintf("failed to get workload GVK from definition ref %s",
			wd.Spec.Reference))
	}
	targetWorkload, err := r.fetchWorkload(targetApp, componentType, gvk)
	if err != nil {
		return nil, nil, err
	}
	if sourceApp == nil {
		return targetWorkload, nil, nil
	}
	// get the component definition in the app,
	sourceWorkload, err := r.fetchWorkload(sourceApp, componentType, gvk)
	if err != nil {
		return nil, nil, err
	}
	return targetWorkload, sourceWorkload, nil
}

// fetchWorkload based on the component type and the application and its gvk
func (r *Reconciler) fetchWorkload(app *corev1alpha2.Application, componentType string,
	gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	// get the component definition in the app,
	comp := app.GetComponent(componentType)
	if comp == nil {
		return nil, fmt.Errorf("cannot find the component %s in the application", componentType)
	}
	// get the workload given GVK and name
	workload, err := oamutil.GetObjectGivenGVKAndName(context.Background(), r, gvk, app.GetNamespace(),
		comp.Name)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get workload %s with gvk %+v ", componentType, gvk))
	}
	return workload, nil
}
