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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/applicationconfiguration"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	appUtil "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev/v1alpha2/applicationrollout"
)

// getTargetApps try to locate the target appRevision and appContext that is responsible for the target
// we will create a new appContext when it's not found
func (r *Reconciler) getTargetApps(ctx context.Context, componentList []string, rollingState v1alpha1.RollingState,
	targetAppRevisionName string) (*v1alpha2.ApplicationRevision, *v1alpha2.ApplicationContext, error) {
	var appRevision v1alpha2.ApplicationRevision
	var appContext v1alpha2.ApplicationContext
	namespaceName := oamutil.GetDefinitionNamespaceWithCtx(ctx)
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: namespaceName, Name: targetAppRevisionName},
		&appRevision); err != nil {
		klog.ErrorS(err, "cannot locate target application revision", "target application revision",
			klog.KRef(namespaceName, targetAppRevisionName))
		return nil, nil, err
	}
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: namespaceName, Name: targetAppRevisionName},
		&appContext); err != nil {
		if apierrors.IsNotFound(err) && rollingState == v1alpha1.LocatingTargetAppState {
			klog.InfoS("target application context does not exist yet, create one", "target application revision",
				klog.KRef(namespaceName, targetAppRevisionName))
			appContext, err = r.createAppContext(ctx, componentList, &appRevision)
			if err != nil {
				return nil, nil, err
			}
			return &appRevision, &appContext, nil
		}
		// the appContext has to exist by now
		klog.ErrorS(err, "cannot locate target application context", "target application name",
			klog.KRef(namespaceName, targetAppRevisionName), "rollingState", rollingState)
		return nil, nil, err
	}
	// special handle the first time we locate the appContext
	if rollingState == v1alpha1.LocatingTargetAppState {
		if appContext.Status.RollingStatus == types.RollingTemplated {
			// force template the target app
			klog.InfoS("force templating an already templated target application",
				"target application revision", klog.KRef(namespaceName, targetAppRevisionName))
			appContext.Status.RollingStatus = types.RollingTemplating
			if err := r.Status().Update(ctx, &appContext); err != nil {
				klog.ErrorS(err, "failed to force update target application context to be templating",
					"target application name", klog.KRef(namespaceName, targetAppRevisionName))
				return nil, nil, err
			}
		}
		err := r.prepareAppContext(ctx, componentList, &appContext)
		if err != nil {
			return nil, nil, err
		}
	}
	return &appRevision, &appContext, nil
}

// getTargetApps try to locate the source appRevision and appContext that is responsible for the source
func (r *Reconciler) getSourceAppContexts(ctx context.Context, componentList []string, rollingState v1alpha1.RollingState,
	sourceAppRevisionName string) (*v1alpha2.ApplicationRevision, *v1alpha2.ApplicationContext, error) {
	var appRevision v1alpha2.ApplicationRevision
	var appContext v1alpha2.ApplicationContext
	namespaceName := oamutil.GetDefinitionNamespaceWithCtx(ctx)
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: namespaceName, Name: sourceAppRevisionName},
		&appRevision); err != nil {
		klog.ErrorS(err, "cannot locate source application revision", "source application revision",
			klog.KRef(namespaceName, sourceAppRevisionName))
		return nil, nil, err
	}
	// the source app has to exist or there is nothing for us to upgrade from
	if err := r.Get(ctx, ktypes.NamespacedName{Namespace: namespaceName, Name: sourceAppRevisionName},
		&appContext); err != nil {
		// TODO: use the app name as the source Context to upgrade from none-rolling application to rolling
		klog.ErrorS(err, "cannot locate source application revision", "source application name",
			klog.KRef(namespaceName, sourceAppRevisionName))
		return nil, nil, err
	}
	// set the AC as rolling if we are still at locating state
	if rollingState == v1alpha1.LocatingTargetAppState {
		err := r.prepareAppContext(ctx, componentList, &appContext)
		if err != nil {
			return nil, nil, err
		}
	}
	return &appRevision, &appContext, nil
}

func (r *Reconciler) prepareAppContext(ctx context.Context, componentList []string,
	appContext *v1alpha2.ApplicationContext) error {
	oamutil.RemoveAnnotations(appContext, []string{oam.AnnotationAppRevision})
	// pass the rolling component to the app
	oamutil.AddAnnotations(appContext, map[string]string{oam.AnnotationAppRollout: strconv.FormatBool(true)})
	if len(componentList) != 0 {
		oamutil.AddAnnotations(appContext, map[string]string{
			oam.AnnotationRollingComponent: strings.Join(componentList, common.RollingComponentsSep)})
	}
	return r.Update(ctx, appContext)
}

func (r *Reconciler) createAppContext(ctx context.Context, componentList []string,
	appRevision *v1alpha2.ApplicationRevision) (v1alpha2.ApplicationContext, error) {
	namespaceName := oamutil.GetDefinitionNamespaceWithCtx(ctx)
	appContext := v1alpha2.ApplicationContext{
		ObjectMeta: metav1.ObjectMeta{
			Name:            appRevision.GetName(),
			Namespace:       namespaceName,
			Labels:          appRevision.GetLabels(),
			Annotations:     appRevision.GetAnnotations(),
			OwnerReferences: appRevision.GetOwnerReferences(),
		},
		Spec: v1alpha2.ApplicationContextSpec{
			ApplicationRevisionName: appRevision.GetName(),
		},
	}
	if metav1.GetControllerOf(&appContext) == nil {
		for i, owner := range appContext.GetOwnerReferences() {
			if owner.Kind == v1alpha2.ApplicationKind {
				appContext.GetOwnerReferences()[i].Controller = pointer.BoolPtr(true)
			}
		}
	}
	// set the AC as rolling
	oamutil.AddAnnotations(&appContext, map[string]string{oam.AnnotationAppRollout: strconv.FormatBool(true)})
	// pass the rolling component to the app
	if len(componentList) != 0 {
		oamutil.AddAnnotations(&appContext, map[string]string{
			oam.AnnotationRollingComponent: strings.Join(componentList, common.RollingComponentsSep)})
	}
	err := r.Create(ctx, &appContext)
	return appContext, err
}

// extractWorkloads extracts the workloads from the source and target applicationConfig
func (r *Reconciler) extractWorkloads(ctx context.Context, componentList []string, targetAppRevision,
	sourceAppRevision *v1alpha2.ApplicationRevision) (*unstructured.Unstructured, *unstructured.Unstructured, error) {
	var componentName string
	var sourceApp *v1alpha2.ApplicationConfiguration
	targetApp, err := oamutil.RawExtension2AppConfig(targetAppRevision.Spec.ApplicationConfiguration)
	if err != nil {
		return nil, nil, err
	}
	if sourceAppRevision != nil {
		sourceApp, err = oamutil.RawExtension2AppConfig(sourceAppRevision.Spec.ApplicationConfiguration)
		if err != nil {
			return nil, nil, err
		}
	}
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
	targetApp *v1alpha2.ApplicationConfiguration) (*unstructured.Unstructured, error) {
	var targetAcc *v1alpha2.ApplicationConfigurationComponent
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
	// inplaceUpgrade not used in rollout now
	applicationconfiguration.SetAppWorkloadInstanceName(componentName, w, revision, "")
	// get the real workload object from api-server given GVK and name
	workload, err := oamutil.GetObjectGivenGVKAndName(ctx, r, w.GroupVersionKind(), targetApp.GetNamespace(), w.GetName())
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get workload %s with gvk %+v ", w.GetName(), w.GroupVersionKind()))
	}

	return workload, nil
}
