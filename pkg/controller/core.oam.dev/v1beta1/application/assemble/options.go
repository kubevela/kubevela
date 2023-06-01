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

package assemble

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
)

// WorkloadOptionFn implement interface WorkloadOption
type WorkloadOptionFn func(*unstructured.Unstructured, *v1beta1.ComponentDefinition, []*unstructured.Unstructured) error

// ApplyToWorkload will apply the manipulation defined in the function to assembled workload
func (fn WorkloadOptionFn) ApplyToWorkload(wl *unstructured.Unstructured,
	compDefinition *v1beta1.ComponentDefinition, packagedWorkloadResources []*unstructured.Unstructured) error {
	return fn(wl, compDefinition, packagedWorkloadResources)
}

// DiscoveryHelmBasedWorkload only works for Helm-based component. It computes a qualifiedFullName for the workload and
// try to get it from K8s cluster.
// If not found, block down-streaming process until Helm creates the workload successfully.
func DiscoveryHelmBasedWorkload(ctx context.Context, c client.Reader) WorkloadOption {
	return WorkloadOptionFn(func(assembledWorkload *unstructured.Unstructured, compDef *v1beta1.ComponentDefinition, resources []*unstructured.Unstructured) error {
		return discoverHelmModuleWorkload(ctx, c, assembledWorkload, compDef, resources)
	})
}

func discoverHelmModuleWorkload(ctx context.Context, c client.Reader, assembledWorkload *unstructured.Unstructured,
	_ *v1beta1.ComponentDefinition, helmResources []*unstructured.Unstructured) error {
	if len(helmResources) == 0 {
		return nil
	}
	if len(assembledWorkload.GetAPIVersion()) == 0 &&
		len(assembledWorkload.GetKind()) == 0 {
		// workload GVK remains to auto-detect
		// we cannot discover workload without GVK, and caller should skip dispatching the assembled workload
		return nil
	}
	ns := assembledWorkload.GetNamespace()
	var rls *unstructured.Unstructured
	for _, v := range helmResources {
		if v.GetKind() == helmapi.HelmReleaseGVK.Kind {
			rls = v.DeepCopy()
			break
		}
	}
	if rls == nil {
		return errors.New("cannot get helm release")
	}
	rlsName := rls.GetName()
	chartName, ok, err := unstructured.NestedString(rls.Object, helmapi.HelmChartNamePath...)
	if err != nil || !ok {
		return errors.New("cannot get helm chart name")
	}

	// qualifiedFullName is used as the name of target workload.
	// It strictly follows the convention that Helm generate default full name as below:
	// > We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
	// > If release name contains chart name it will be used as a full name.
	qualifiedWorkloadName := rlsName
	if !strings.Contains(rlsName, chartName) {
		qualifiedWorkloadName = fmt.Sprintf("%s-%s", rlsName, chartName)
		if len(qualifiedWorkloadName) > 63 {
			qualifiedWorkloadName = strings.TrimSuffix(qualifiedWorkloadName[:63], "-")
		}
	}

	workloadByHelm := assembledWorkload.DeepCopy()
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: qualifiedWorkloadName}, workloadByHelm); err != nil {
		return err
	}

	// check it's created by helm and match the release info
	annots := workloadByHelm.GetAnnotations()
	labels := workloadByHelm.GetLabels()
	if annots == nil || labels == nil ||
		annots["meta.helm.sh/release-name"] != rlsName ||
		annots["meta.helm.sh/release-namespace"] != ns ||
		labels["app.kubernetes.io/managed-by"] != "Helm" {
		err := fmt.Errorf("the workload is found but not match with helm info(meta.helm.sh/release-name: %s, meta.helm.sh/namespace: %s, app.kubernetes.io/managed-by: Helm)", rlsName, ns)
		klog.ErrorS(err, "Found a name-matched workload but not managed by Helm", "name", qualifiedWorkloadName,
			"annotations", annots, "labels", labels)
		return err
	}
	assembledWorkload.SetName(qualifiedWorkloadName)
	return nil
}
