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
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	mapset "github.com/deckarep/golang-set"
	"github.com/mitchellh/hashstructure/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
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
var allBuiltinCapabilities = mapset.NewSet(common.MetricsControllerName, common.PodspecWorkloadControllerName,
	common.RouteControllerName, common.AutoscaleControllerName)

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
func GetAppNextRevision(app *v1alpha2.Application) (string, int64) {
	if app == nil {
		// should never happen
		return "", 0
	}
	var nextRevision int64 = 1
	if app.Status.LatestRevision != nil {
		// we only bump the version when we are rolling
		if _, exist := app.GetAnnotations()[oam.AnnotationAppRollout]; exist {
			nextRevision = app.Status.LatestRevision.Revision + 1
		} else {
			nextRevision = app.Status.LatestRevision.Revision
		}
	}
	return ConstructRevisionName(app.Name, nextRevision), nextRevision
}

// ConstructRevisionName will generate a revisionName given the componentName and revision
// will be <componentName>-v<RevisionNumber>, for example: comp-v1
func ConstructRevisionName(componentName string, revision int64) string {
	return strings.Join([]string{componentName, fmt.Sprintf("v%d", revision)}, "-")
}

// ExtractComponentName will extract the componentName from a revisionName
func ExtractComponentName(revisionName string) string {
	splits := strings.Split(revisionName, "-")
	return strings.Join(splits[0:len(splits)-1], "-")
}

// ExtractRevision will extract the revision from a revisionName
func ExtractRevision(revisionName string) (int, error) {
	splits := strings.Split(revisionName, "-")
	// the revision is the last string without the prefix "v"
	return strconv.Atoi(strings.TrimPrefix(splits[len(splits)-1], "v"))
}

// CompareWithRevision compares a component's spec with the component's latest revision content
func CompareWithRevision(ctx context.Context, c client.Client, logger logging.Logger, componentName, nameSpace,
	latestRevision string, curCompSpec *v1alpha2.ComponentSpec) (bool, error) {
	oldRev := &appsv1.ControllerRevision{}
	// retry on NotFound since we update the component last revision first
	err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		err := c.Get(ctx, client.ObjectKey{Namespace: nameSpace, Name: latestRevision}, oldRev)
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("get old controllerRevision %s error %v",
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
		logger.Info(fmt.Sprintf("Unmarshal old controllerRevision %s error %v",
			latestRevision, err), "componentName", componentName)
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
