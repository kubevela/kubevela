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

package resourcetracker

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

const (
	// Finalizer for resourcetracker to clean up recorded resources
	Finalizer = "resourcetracker.core.oam.dev/finalizer"
)

func setCompression(rt *v1beta1.ResourceTracker) {
	if t := compressionType(); t != "" {
		rt.Spec.Compression.Type = t
	}
}

// RecordManifestsInResourceTracker records resources in ResourceTracker
func RecordManifestsInResourceTracker(
	ctx context.Context,
	cli client.Client,
	rt *v1beta1.ResourceTracker,
	manifests []*unstructured.Unstructured,
	metaOnly bool,
	skipGC bool,
	creator string) error {
	if len(manifests) == 0 {
		return nil
	}
	objs := make([]client.Object, 0, len(manifests))
	for _, manifest := range manifests {
		objs = append(objs, manifest)
	}
	if !rt.AddManagedResources(objs, metaOnly, skipGC, creator) {
		return nil // nothing recorded changed, so the tracker does not need writing
	}
	return cli.Update(ctx, rt)
}

// DeletedManifestInResourceTracker marks resources as deleted in resourcetracker, if remove is true, resources will be removed from resourcetracker
func DeletedManifestInResourceTracker(ctx context.Context, cli client.Client, rt *v1beta1.ResourceTracker, manifest *unstructured.Unstructured, remove bool) error {
	if updated := rt.DeleteManagedResource(manifest, remove); !updated {
		return nil
	}
	return cli.Update(ctx, rt)
}
