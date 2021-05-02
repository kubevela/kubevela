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

package appdeployment

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	ocmv1work "github.com/open-cluster-management/api/work/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func (r *Reconciler) applyRevisionsByOCM(ctx context.Context, appd *oamcore.AppDeployment) error {
	for _, rev := range appd.Spec.AppRevisions {
		workloads, err := r.getWorkloadsFromRevision(ctx, rev.RevisionName, appd.Namespace)
		if err != nil {
			return err
		}
		for _, placement := range rev.Placement {
			clusterName := placement.ClusterSelector.Name // TODO: Deal w/ NPE
			if err := r.Client.Get(ctx, types.NamespacedName{Name: clusterName}, &corev1.Namespace{}); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			manifestWork := convertManifestWork(workloads, clusterName, rev.RevisionName)
			const (
				fieldManagerKubeVelaAppDeployment = "kube-vela-appdeployment-operator"
			)
			existingManifestWork := &ocmv1work.ManifestWork{}
			err := r.Client.Get(ctx, types.NamespacedName{
				Namespace: manifestWork.Namespace,
				Name:      manifestWork.Name,
			}, existingManifestWork)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					return err
				}
			} else {
				manifestWork.Status = existingManifestWork.Status
			}

			if err := r.Client.Patch(ctx, manifestWork, client.Apply, &client.PatchOptions{
				FieldManager: fieldManagerKubeVelaAppDeployment,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) deleteRevisionsByOCM(ctx context.Context, appd *oamcore.AppDeployment) (err error) {
	for _, rev := range appd.Spec.AppRevisions {
		workloads, err := r.getWorkloadsFromRevision(ctx, rev.RevisionName, appd.Namespace)
		if err != nil {
			return err
		}
		for _, placement := range rev.Placement {
			clusterName := placement.ClusterSelector.Name // TODO: Deal w/ NPE
			manifestWork := convertManifestWork(workloads, clusterName, rev.RevisionName)
			if err := r.Client.Delete(ctx, manifestWork); err != nil {
				return err
			}
		}
	}
	return nil
}

func convertManifestWork(workloads []*workload, clusterName string, revName string) *ocmv1work.ManifestWork {
	manifests := make([]ocmv1work.Manifest, len(workloads)) // TODO: stablize manifests element order
	for i, workload := range workloads {
		workload := workload
		manifests[i] = ocmv1work.Manifest{
			RawExtension: runtime.RawExtension{
				Object: workload.Object,
			},
		}
		for _, trait := range workload.traits {
			trait := trait
			manifests = append(manifests, ocmv1work.Manifest{
				RawExtension: runtime.RawExtension{
					Object: trait.Object,
				},
			})
		}
	}
	manifestWork := &ocmv1work.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ocmv1work.GroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterName, // Namespace is the target cluster name
			Name:      revName,     // TODO: Name length validation
		},
		Spec: ocmv1work.ManifestWorkSpec{
			Workload: ocmv1work.ManifestsTemplate{
				Manifests: manifests,
			},
		},
	}
	return manifestWork
}
