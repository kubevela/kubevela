/*
Copyright 2026 The KubeVela Authors.

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

package addon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// serverSetMetadataFields are metadata fields the API server populates. They
// must be stripped before storing a manifest in the drift tracker, otherwise a
// stale resourceVersion (or other server-set field) would make the steady-state
// re-apply 409-conflict on every heal.
var serverSetMetadataFields = []string{
	"resourceVersion",
	"uid",
	"creationTimestamp",
	"generation",
	"managedFields",
	"selfLink",
	// Lifecycle fields: a manifest captured from a live object that was
	// mid-deletion would otherwise carry these, and re-applying it fails with
	// "field is immutable" (deletionTimestamp/deletionGracePeriodSeconds) or
	// re-introduces stale finalizers. A stored desired manifest must have none.
	"deletionTimestamp",
	"deletionGracePeriodSeconds",
	"finalizers",
}

// trackerOwnedByAddonLabel marks the addon-owned drift ResourceTracker. It is
// deliberately NOT one of the Application GC labels (oam.LabelAppName /
// oam.LabelAppNamespace), so the Application ResourceTracker garbage collector,
// which selects only by those labels, never claims this tracker.
const trackerOwnedByAddonLabel = "addon.oam.dev/owned-by"

// trackerName is the name of the addon-owned ResourceTracker that records the
// resources an addon manages for drift detection. It is distinct from the
// Application's own ResourceTrackers.
func trackerName(addonName string) string { return "addon-" + addonName + "-drift" }

// managedResourceFrom serializes a desired object into a ManagedResource
// (object reference plus the full manifest in raw) for storage in the tracker.
// Server-populated fields (resourceVersion, uid, status, managedFields, ...) are
// stripped: the installer emits post-apply (live) objects, and re-applying a
// manifest that still carries a stale resourceVersion would 409-conflict on
// every steady-state heal.
func managedResourceFrom(obj client.Object) (v1beta1.ManagedResource, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return v1beta1.ManagedResource{}, err
	}
	// Ensure the cleaned manifest keeps its type identity even if the source
	// object had it only on the typed struct.
	u["apiVersion"] = gvk.GroupVersion().String()
	u["kind"] = gvk.Kind
	// Drop server-managed metadata fields.
	if meta, ok, _ := unstructured.NestedMap(u, "metadata"); ok {
		for _, f := range serverSetMetadataFields {
			delete(meta, f)
		}
		u["metadata"] = meta
	}
	// Drop the server-reconciled status subresource.
	delete(u, "status")

	raw, err := json.Marshal(u)
	if err != nil {
		return v1beta1.ManagedResource{}, err
	}
	mr := v1beta1.ManagedResource{}
	mr.APIVersion = gvk.GroupVersion().String()
	mr.Kind = gvk.Kind
	mr.Namespace = obj.GetNamespace()
	mr.Name = obj.GetName()
	mr.Data = &runtime.RawExtension{Raw: raw}
	return mr, nil
}

// writeTracker creates or updates the addon's drift ResourceTracker with the
// given objects' manifests. The RT is owned by the Addon CR and carries the
// owned-by label so the Application RT garbage collector never claims it.
func (r *Reconciler) writeTracker(ctx context.Context, ad *v1beta1.Addon, objs []client.Object) error {
	if len(objs) == 0 {
		// A tracker with no managed resources silently disables drift detection
		// while looking healthy. Refuse to write it so the caller surfaces the
		// install problem instead.
		return fmt.Errorf("addon %s: installer emitted no objects; refusing to write empty drift tracker", ad.Name)
	}
	mrs := make([]v1beta1.ManagedResource, 0, len(objs))
	for _, o := range objs {
		mr, err := managedResourceFrom(o)
		if err != nil {
			return err
		}
		mrs = append(mrs, mr)
	}
	rt := &v1beta1.ResourceTracker{ObjectMeta: metav1.ObjectMeta{Name: trackerName(ad.Name)}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rt, func() error {
		if rt.Labels == nil {
			rt.Labels = map[string]string{}
		}
		rt.Labels[trackerOwnedByAddonLabel] = ad.Name
		rt.Spec.ManagedResources = mrs
		return controllerutil.SetControllerReference(ad, rt, r.Scheme)
	})
	return err
}

// loadTracker returns the addon's drift ResourceTracker, or nil if absent.
func (r *Reconciler) loadTracker(ctx context.Context, addonName string) (*v1beta1.ResourceTracker, error) {
	rt := &v1beta1.ResourceTracker{}
	if err := r.Get(ctx, client.ObjectKey{Name: trackerName(addonName)}, rt); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return rt, nil
}

// healFromTracker re-applies every managed resource from the tracker, mirroring
// the Application controller's StateKeep: a deleted resource is recreated, an
// edited resource is patched back to the stored manifest, an unchanged resource
// no-ops. It never fetches from the registry. Errors are aggregated so one bad
// resource does not block the rest.
func (r *Reconciler) healFromTracker(ctx context.Context, rt *v1beta1.ResourceTracker) error {
	if rt == nil {
		return nil
	}
	applicator := apply.NewAPIApplicator(r.Client)
	var errs []error

	// Pass 1: re-apply the owning Application first and read back its live UID.
	// Auxiliaries carry an ownerReference to the Application; if the Application
	// was deleted and recreated, that stored UID is stale, and Kubernetes would
	// garbage-collect any auxiliary we re-create with it (owner not found).
	// Applying the app first lets pass 2 re-stamp the owner UID to the live one.
	var appName string
	var appUID ktypes.UID
	var hadApp bool
	for i := range rt.Spec.ManagedResources {
		mr := rt.Spec.ManagedResources[i]
		if mr.Kind != "Application" {
			continue
		}
		hadApp = true
		obj, err := decodeManaged(mr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if obj == nil {
			continue
		}
		if err := applicator.Apply(ctx, obj, apply.DisableUpdateAnnotation()); err != nil {
			errs = append(errs, fmt.Errorf("re-apply %s: %w", mr.ResourceKey(), err))
			continue
		}
		// Apply does not reliably populate UID on the patch path, so read it back.
		// The live UID is required to re-stamp auxiliary owner references in pass 2.
		// If the read-back fails, do not silently drop the error: leaving appUID
		// empty would make pass 2 re-apply auxiliaries carrying the stale stored
		// owner UID, which Kubernetes would then garbage-collect (owner not found) —
		// the exact churn this two-pass design exists to prevent.
		live := &unstructured.Unstructured{}
		live.SetGroupVersionKind(obj.GroupVersionKind())
		if gerr := r.Get(ctx, client.ObjectKeyFromObject(obj), live); gerr != nil {
			errs = append(errs, fmt.Errorf("read back %s for owner re-stamp: %w", mr.ResourceKey(), gerr))
			continue
		}
		appName, appUID = live.GetName(), live.GetUID()
	}

	// An Application is recorded but its live UID could not be resolved this
	// reconcile (apply or read-back failed above). Re-applying the auxiliaries now
	// would stamp them with the stale stored owner UID and they would be garbage-
	// collected. Skip pass 2 and requeue via the aggregated error instead.
	if hadApp && appUID == "" {
		return errors.Join(errs...)
	}

	// Pass 2: re-apply the auxiliaries, re-stamping any ownerReference to the
	// Application onto its current UID before applying.
	for i := range rt.Spec.ManagedResources {
		mr := rt.Spec.ManagedResources[i]
		if mr.Kind == "Application" {
			continue
		}
		obj, err := decodeManaged(mr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if obj == nil {
			continue
		}
		if appUID != "" {
			restampAppOwnerUID(obj, appName, appUID)
		}
		if err := applicator.Apply(ctx, obj, apply.DisableUpdateAnnotation()); err != nil {
			errs = append(errs, fmt.Errorf("re-apply %s: %w", mr.ResourceKey(), err))
		}
	}
	return errors.Join(errs...)
}

// decodeManaged decodes a managed resource's stored manifest, returning nil when
// no manifest is recorded. It unmarshals directly (rather than via
// ToUnstructuredWithData, which swallows non-missing-data errors and could yield
// a near-empty object that would strip live fields on apply).
func decodeManaged(mr v1beta1.ManagedResource) (*unstructured.Unstructured, error) {
	if mr.Data == nil || mr.Data.Raw == nil {
		return nil, nil
	}
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(mr.Data.Raw, &obj.Object); err != nil {
		return nil, fmt.Errorf("decode %s: %w", mr.ResourceKey(), err)
	}
	return obj, nil
}

// restampAppOwnerUID rewrites any ownerReference that points at the addon's
// Application (matched by kind and name) so it carries the live UID. Without
// this, an auxiliary re-applied after the Application was recreated would carry a
// stale owner UID and be garbage-collected immediately.
func restampAppOwnerUID(obj *unstructured.Unstructured, appName string, appUID ktypes.UID) {
	if appName == "" {
		return
	}
	refs := obj.GetOwnerReferences()
	changed := false
	for i := range refs {
		if refs[i].Kind == "Application" && refs[i].Name == appName {
			refs[i].UID = appUID
			changed = true
		}
	}
	if changed {
		obj.SetOwnerReferences(refs)
	}
}
