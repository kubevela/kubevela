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
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/hashstructure/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/workflow/pkg/cue/packages"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// DefaultBackoff is the backoff we use in controller
var DefaultBackoff = wait.Backoff{
	Duration: 1 * time.Second,
	Factor:   2,
	Steps:    5,
	Jitter:   0.1,
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
func RefreshPackageDiscover(ctx context.Context, k8sClient client.Client, pd *packages.PackageDiscover, definition runtime.Object) error {
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
			gvk, err = util.GetGVKFromDefinition(k8sClient.RESTMapper(), workloadDef.Spec.Reference)
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
		gvk, err = util.GetGVKFromDefinition(k8sClient.RESTMapper(), def.Spec.Reference)
		if err != nil {
			return err
		}
	case *v1beta1.PolicyDefinition:
		gvk, err = util.GetGVKFromDefinition(k8sClient.RESTMapper(), def.Spec.Reference)
		if err != nil {
			return err
		}
	case *v1beta1.WorkflowStepDefinition:
		gvk, err = util.GetGVKFromDefinition(k8sClient.RESTMapper(), def.Spec.Reference)
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
