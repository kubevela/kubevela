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

package applicationconfiguration

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/dependency/kruiseapi"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// below are the resources that we know how to disable
	cloneSetDisablePath            = "spec.updateStrategy.paused"
	advancedStatefulSetDisablePath = "spec.updateStrategy.rollingUpdate.paused"
	deploymentDisablePath          = "spec.paused"
)

// SetAppWorkloadInstanceName sets the name of the workload instance depends on the component revision
// and the workload kind
func SetAppWorkloadInstanceName(componentName string, w *unstructured.Unstructured, revision int, inplaceUpgrade string) {

	if inplaceUpgrade == strconv.FormatBool(true) {
		klog.InfoS("we reuse the component name for resources that support in-place upgrade",
			"GVK", w.GroupVersionKind(), "instance name", componentName, oam.AnnotationInplaceUpgrade, true)
		w.SetName(componentName)
		return
	}

	// we hard code the behavior depends on the workload group/kind for now. The only in-place upgradable resources
	// we support is cloneset/statefulset for now. We can easily add more later.
	if w.GroupVersionKind().Group == kruiseapi.GroupVersion.Group {
		if w.GetKind() == kruiseapi.CloneSet ||
			w.GetKind() == kruiseapi.StatefulSet {
			// we use the component name alone for those resources that do support in-place upgrade
			klog.InfoS("we reuse the component name for resources that support in-place upgrade",
				"GVK", w.GroupVersionKind(), "instance name", componentName)
			w.SetName(componentName)
			return
		}
	}
	// we assume that the rest of the resources do not support in-place upgrade
	instanceName := utils.ConstructRevisionName(componentName, int64(revision))
	klog.InfoS("we encountered an unknown resources, assume that it does not support in-place upgrade",
		"GVK", w.GroupVersionKind(), "instance name", instanceName)
	w.SetName(instanceName)

}

// prepWorkloadInstanceForRollout prepare the workload before it is emit to the k8s. The current approach is to mark it
// as disabled so that it's spec won't take effect immediately. The rollout controller can take over the resources
// and enable it on its own since appConfig controller here won't override their change
func prepWorkloadInstanceForRollout(workload *unstructured.Unstructured) error {
	pv := fieldpath.Pave(workload.UnstructuredContent())

	// TODO: we can get the workloadDefinition name from workload.GetLabels()["oam.WorkloadTypeLabel"]
	// and use a special field like "disablePath" in the definition to allow configurable behavior

	// we hard code the behavior depends on the known workload group/kind for now.
	if workload.GroupVersionKind().Group == kruiseapi.GroupVersion.Group {
		switch workload.GetKind() {
		case kruiseapi.CloneSet:
			err := pv.SetBool(cloneSetDisablePath, true)
			if err != nil {
				return err
			}
			klog.InfoS("we render a CloneSet workload paused on the first time",
				"kind", workload.GetKind(), "instance name", workload.GetName())
			return nil
		case kruiseapi.StatefulSet:
			err := pv.SetBool(advancedStatefulSetDisablePath, true)
			if err != nil {
				return err
			}
			klog.InfoS("we render an advanced statefulset workload paused on the first time",
				"kind", workload.GetKind(), "instance name", workload.GetName())
			return nil
		}
	} else if workload.GroupVersionKind().Group == appsv1.GroupName &&
		workload.GetKind() == reflect.TypeOf(appsv1.Deployment{}).Name() {
		err := pv.SetBool(deploymentDisablePath, true)
		if err != nil {
			return err
		}
		klog.InfoS("we render a deployment workload paused on the first time",
			"kind", workload.GetKind(), "instance name", workload.GetName())
		return nil
	}
	klog.InfoS("we encountered an unknown resource, we don't know how to prepare it",
		"GVK", workload.GroupVersionKind().String(), "instance name", workload.GetName())
	return fmt.Errorf("we do not know how to prepare `%s` as it has an unknown type %s", workload.GetName(),
		workload.GroupVersionKind().String())
}
