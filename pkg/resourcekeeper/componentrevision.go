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

	"github.com/pkg/errors"
	v1 "k8s.io/api/apps/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// DispatchComponentRevision create component revision (also add record in resourcetracker)
func (h *resourceKeeper) DispatchComponentRevision(ctx context.Context, cr *v1.ControllerRevision) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	rt, err := h.getComponentRevisionRT(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get resourcetracker")
	}
	obj := &unstructured.Unstructured{}
	obj.SetName(cr.Name)
	obj.SetNamespace(cr.Namespace)
	obj.SetLabels(cr.Labels)
	if err = resourcetracker.RecordManifestsInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, []*unstructured.Unstructured{obj}, true, false, common.WorkflowResourceCreator); err != nil {
		return errors.Wrapf(err, "failed to record componentrevision %s/%s/%s", oam.GetCluster(cr), cr.Namespace, cr.Name)
	}
	if err = h.Client.Create(auth.ContextWithUserInfo(multicluster.ContextWithClusterName(ctx, oam.GetCluster(cr)), h.app), cr); err != nil {
		return errors.Wrapf(err, "failed to create componentrevision %s/%s/%s", oam.GetCluster(cr), cr.Namespace, cr.Name)
	}
	return nil
}

// DeleteComponentRevision delete component revision (also remove record in resourcetracker)
func (h *resourceKeeper) DeleteComponentRevision(ctx context.Context, cr *v1.ControllerRevision) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	rt, err := h.getComponentRevisionRT(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get resourcetracker")
	}
	obj := &unstructured.Unstructured{}
	obj.SetName(cr.Name)
	obj.SetNamespace(cr.Namespace)
	obj.SetLabels(cr.Labels)
	if err = h.Client.Delete(auth.ContextWithUserInfo(multicluster.ContextWithClusterName(ctx, oam.GetCluster(cr)), h.app), cr); err != nil && !errors2.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete componentrevision %s/%s/%s", oam.GetCluster(cr), cr.Namespace, cr.Name)
	}
	if err = resourcetracker.DeletedManifestInResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, rt, obj, true); err != nil {
		return errors.Wrapf(err, "failed to componentrevision resourcetracker record %s/%s/%s", oam.GetCluster(cr), cr.Namespace, cr.Name)
	}
	return nil
}
