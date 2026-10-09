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

package resourcekeeper

import (
	"context"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// DeleteOption option for delete
type DeleteOption interface {
	ApplyToDeleteConfig(*deleteConfig)
}

type deleteConfig struct {
	rtConfig
}

func newDeleteConfig(options ...DeleteOption) *deleteConfig {
	cfg := &deleteConfig{}
	for _, option := range options {
		option.ApplyToDeleteConfig(cfg)
	}
	return cfg
}

// Delete delete resources
func (h *resourceKeeper) Delete(ctx context.Context, manifests []*unstructured.Unstructured, options ...DeleteOption) (err error) {
	h.ClearNamespaceForClusterScopedResources(manifests)
	if err = h.AdmissionCheck(ctx, manifests); err != nil {
		return err
	}
	for _, manifest := range manifests {
		if manifest != nil {
			cfg := newDeleteConfig(options...)
			if err = h.delete(ctx, manifest, cfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *resourceKeeper) delete(ctx context.Context, manifest *unstructured.Unstructured, cfg *deleteConfig) (err error) {
	clusterCtx := pkgmulticluster.WithCluster(ctx, oam.GetCluster(manifest))
	deleteCtx := h.asRequester(clusterCtx)

	// A caller can pass only the identity of a resource, which is all a workflow kube.#Delete step
	// has. What protects the resource, such as its sharer list and the labels a garbage-collect rule
	// selects on, is on the live object, so that is what the checks below run against. It is read as
	// the keeper, so the requester needs no permission to get, but the keeper must be able to read it.
	// If it cannot, the protections cannot be checked, and nothing is deleted.
	live := &unstructured.Unstructured{}
	live.SetGroupVersionKind(manifest.GroupVersionKind())
	if err = h.Client.Get(asSelf(clusterCtx), client.ObjectKeyFromObject(manifest), live); err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "cannot get manifest, name: %s apiVersion: %s kind: %s", manifest.GetName(), manifest.GetAPIVersion(), manifest.GetKind())
		}
		live = nil
	}
	target := manifest
	if live != nil {
		target = live
	}
	if h.policies.GarbageCollect != nil {
		if strategy := h.policies.GarbageCollect.FindStrategy(target); strategy != nil {
			GarbageCollectStrategyOption(*strategy).ApplyToDeleteConfig(cfg)
		}
	}

	// 1. mark manifests as deleted in resourcetracker
	var rt *v1beta1.ResourceTracker
	if cfg.useRoot || cfg.skipGC {
		rt, err = h.getRootRT(ctx)
	} else {
		rt, err = h.getCurrentRT(ctx)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to get resourcetracker")
	}
	if err = resourcetracker.DeletedManifestInResourceTracker(localCluster(ctx), h.Client, rt, manifest, false); err != nil {
		return errors.Wrapf(err, "failed to delete resources in resourcetracker")
	}
	if live == nil {
		return nil
	}

	// 2. let the owner go of the live object the way garbage collection does: a resource still
	// shared with another owner is handed over, one that is to be kept is released, and the rest
	// is deleted
	mr := v1beta1.ManagedResource{
		ClusterObjectReference: common.ClusterObjectReference{
			Cluster: oam.GetCluster(manifest),
			ObjectReference: corev1.ObjectReference{
				APIVersion: live.GetAPIVersion(),
				Kind:       live.GetKind(),
				Namespace:  live.GetNamespace(),
				Name:       live.GetName(),
			},
		},
		SkipGC: cfg.skipGC,
	}
	return DeleteManagedResource(deleteCtx, h.Client, mr, live, h.owner, h.policies.GarbageCollect)
}
