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

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	mapset "github.com/deckarep/golang-set"
	"github.com/mitchellh/hashstructure/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/cue/packages"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// DefaultBackoff is the backoff we use in controller
var DefaultBackoff = wait.Backoff{
	Duration: 1 * time.Second,
	Factor:   2,
	Steps:    5,
	Jitter:   0.1,
}

// LabelPodSpecable defines whether a workload has podSpec or not.
const LabelPodSpecable = "workload.oam.dev/podspecable"

// allBuiltinCapabilities includes all builtin controllers
// TODO(zzxwill) needs to automatically discovery all controllers
var allBuiltinCapabilities = mapset.NewSet(
	common.RolloutControllerName,
	common.HealthScopeControllerName,
	common.EnvBindingControllerName,
)

// GetPodSpecPath get podSpec field and label
func GetPodSpecPath(workloadDef *v1alpha2.WorkloadDefinition) (string, bool) {
	if workloadDef.Spec.PodSpecPath != "" {
		return workloadDef.Spec.PodSpecPath, true
	}
	if workloadDef.Labels == nil {
		return "", false
	}
	podSpecable, ok := workloadDef.Labels[LabelPodSpecable]
	if !ok {
		return "", false
	}
	ok, _ = strconv.ParseBool(podSpecable)
	return "", ok
}

// DiscoveryFromPodSpec will discover pods from podSpec
func DiscoveryFromPodSpec(w *unstructured.Unstructured, fieldPath string) ([]intstr.IntOrString, error) {
	paved := fieldpath.Pave(w.Object)
	obj, err := paved.GetValue(fieldPath)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("discovery podSpec from %s in workload %v err %w", fieldPath, w.GetName(), err)
	}
	var spec corev1.PodSpec
	err = json.Unmarshal(data, &spec)
	if err != nil {
		return nil, fmt.Errorf("discovery podSpec from %s in workload %v err %w", fieldPath, w.GetName(), err)
	}
	ports := getContainerPorts(spec.Containers)
	if len(ports) == 0 {
		return nil, fmt.Errorf("no port found in podSpec %v", w.GetName())
	}
	return ports, nil
}

// DiscoveryFromPodTemplate not only discovery port, will also use labels in podTemplate
func DiscoveryFromPodTemplate(w *unstructured.Unstructured, fields ...string) ([]intstr.IntOrString, map[string]string, error) {
	obj, found, _ := unstructured.NestedMap(w.Object, fields...)
	if !found {
		return nil, nil, fmt.Errorf("not have spec.template in workload %v", w.GetName())
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, nil, fmt.Errorf("workload %v convert object err %w", w.GetName(), err)
	}
	var spec corev1.PodTemplateSpec
	err = json.Unmarshal(data, &spec)
	if err != nil {
		return nil, nil, fmt.Errorf("workload %v convert object to PodTemplate err %w", w.GetName(), err)
	}
	ports := getContainerPorts(spec.Spec.Containers)
	if len(ports) == 0 {
		return nil, nil, fmt.Errorf("no port found in workload %v", w.GetName())
	}
	return ports, spec.Labels, nil
}

func getContainerPorts(cs []corev1.Container) []intstr.IntOrString {
	var ports []intstr.IntOrString
	// TODO(wonderflow): exclude some sidecars
	for _, container := range cs {
		for _, port := range container.Ports {
			ports = append(ports, intstr.FromInt(int(port.ContainerPort)))
		}
	}
	return ports
}

// SelectOAMAppLabelsWithoutRevision will filter and return OAM app labels only, if no labels, return the original one.
func SelectOAMAppLabelsWithoutRevision(labels map[string]string) map[string]string {
	newLabel := make(map[string]string)
	for k, v := range labels {
		// Note: we don't include revision label by design
		// if we want to distinguish with different revisions, we should include it in other function.
		if k != oam.LabelAppName && k != oam.LabelAppComponent {
			continue
		}
		newLabel[k] = v
	}
	if len(newLabel) == 0 {
		return labels
	}
	return newLabel
}

// CheckDisabledCapabilities checks whether the disabled capability controllers are valid
func CheckDisabledCapabilities(disableCaps string) error {
	switch disableCaps {
	case common.DisableNoneCaps:
		return nil
	case common.DisableAllCaps:
		return nil
	default:
		for _, c := range strings.Split(disableCaps, ",") {
			if !allBuiltinCapabilities.Contains(c) {
				return fmt.Errorf("%s in disable caps list is not built-in", c)
			}
		}
		return nil
	}
}

// StoreInSet stores items in Set
func StoreInSet(disableCaps string) mapset.Set {
	var disableSlice []interface{}
	for _, c := range strings.Split(disableCaps, ",") {
		disableSlice = append(disableSlice, c)
	}
	return mapset.NewSetFromSlice(disableSlice)
}

// GetAppNextRevision will generate the next revision name and revision number for application
func GetAppNextRevision(app *v1beta1.Application) (string, int64) {
	if app == nil {
		// should never happen
		return "", 0
	}
	var nextRevision int64 = 1
	if app.Status.LatestRevision != nil {
		// revision will always bump and increment no matter what the way user is running.
		nextRevision = app.Status.LatestRevision.Revision + 1
	}
	return ConstructRevisionName(app.Name, nextRevision), nextRevision
}

// ConstructRevisionName will generate a revisionName given the componentName and revision
// will be <componentName>-v<RevisionNumber>, for example: comp-v1
func ConstructRevisionName(componentName string, revision int64) string {
	return strings.Join([]string{componentName, fmt.Sprintf("v%d", revision)}, "-")
}

// ExtractComponentName will extract the componentName from a revisionName
var ExtractComponentName = util.ExtractComponentName

// ExtractRevision will extract the revision from a revisionName
func ExtractRevision(revisionName string) (int, error) {
	splits := strings.Split(revisionName, "-")
	// the revision is the last string without the prefix "v"
	return strconv.Atoi(strings.TrimPrefix(splits[len(splits)-1], "v"))
}

// CompareWithRevision compares a component's spec with the component's latest revision content
func CompareWithRevision(ctx context.Context, c client.Client, componentName, nameSpace,
	latestRevision string, curCompSpec *v1alpha2.ComponentSpec) (bool, error) {
	oldRev := &appsv1.ControllerRevision{}
	// retry on NotFound since we update the component last revision first
	err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		err := c.Get(ctx, client.ObjectKey{Namespace: nameSpace, Name: latestRevision}, oldRev)
		if err != nil && !kerrors.IsNotFound(err) {
			klog.InfoS(fmt.Sprintf("get old controllerRevision %s error %v",
				latestRevision, err), "componentName", componentName)
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return true, err
	}
	oldComp, err := util.UnpackRevisionData(oldRev)
	if err != nil {
		klog.InfoS("Unmarshal old controllerRevision", latestRevision, "error", err, "componentName", componentName)
		return true, err
	}
	if reflect.DeepEqual(curCompSpec, &oldComp.Spec) {
		// no need to create a new revision
		return false, nil
	}
	return true, nil
}

// ComputeSpecHash computes the hash value of a k8s resource spec
func ComputeSpecHash(spec interface{}) (string, error) {
	// compute a hash value of any resource spec
	specHash, err := hashstructure.Hash(spec, hashstructure.FormatV2, nil)
	if err != nil {
		return "", err
	}
	specHashLabel := strconv.FormatUint(specHash, 16)
	return specHashLabel, nil
}

// RefreshPackageDiscover help refresh package discover
// Deprecated: The function RefreshKubePackagesFromCluster affects performance and the code has been commented a long time.
func RefreshPackageDiscover(ctx context.Context, k8sClient client.Client, dm discoverymapper.DiscoveryMapper,
	pd *packages.PackageDiscover, definition runtime.Object) error {
	var gvk metav1.GroupVersionKind
	var err error
	switch def := definition.(type) {
	case *v1beta1.ComponentDefinition:
		if def.Spec.Workload.Definition == (commontypes.WorkloadGVK{}) {
			workloadDef := new(v1beta1.WorkloadDefinition)
			err = k8sClient.Get(ctx, client.ObjectKey{Name: def.Spec.Workload.Type, Namespace: def.Namespace}, workloadDef)
			if err != nil {
				return err
			}
			gvk, err = util.GetGVKFromDefinition(dm, workloadDef.Spec.Reference)
			if err != nil {
				return err
			}
		} else {
			gv, err := schema.ParseGroupVersion(def.Spec.Workload.Definition.APIVersion)
			if err != nil {
				return err
			}
			gvk = metav1.GroupVersionKind{
				Group:   gv.Group,
				Version: gv.Version,
				Kind:    def.Spec.Workload.Definition.Kind,
			}
		}
	case *v1beta1.TraitDefinition:
		gvk, err = util.GetGVKFromDefinition(dm, def.Spec.Reference)
		if err != nil {
			return err
		}
	case *v1beta1.PolicyDefinition:
		gvk, err = util.GetGVKFromDefinition(dm, def.Spec.Reference)
		if err != nil {
			return err
		}
	case *v1beta1.WorkflowStepDefinition:
		gvk, err = util.GetGVKFromDefinition(dm, def.Spec.Reference)
		if err != nil {
			return err
		}
	default:
	}
	targetGVK := metav1.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
	if exist := pd.Exist(targetGVK); exist {
		return nil
	}

	if err := pd.RefreshKubePackagesFromCluster(); err != nil {
		return err
	}

	// Test whether the refresh is successful
	// if exist := pd.Exist(targetGVK); !exist {
	//	 return fmt.Errorf("get CRD %s error", targetGVK.String())
	// }
	return nil
}

// GetUnstructuredObjectStatusCondition returns the status.condition with matching condType from an unstructured object.
func GetUnstructuredObjectStatusCondition(obj *unstructured.Unstructured, condType string) (*condition.Condition, bool, error) {
	cs, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	for _, c := range cs {
		b, err := json.Marshal(c)
		if err != nil {
			return nil, false, err
		}
		condObj := &condition.Condition{}
		err = json.Unmarshal(b, condObj)
		if err != nil {
			return nil, false, err
		}

		if string(condObj.Type) != condType {
			continue
		}
		return condObj, true, nil
	}

	return nil, false, nil
}
