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

package application

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// holdForGlobalPolicy keeps app at rev while a mandatory global fails: the reconcile goes on with
// the revision's spec and the policy context last applied, and no new revision is cut until the
// global renders again. It returns ctx carrying that context, or err when the app cannot be held.
func (h *AppHandler) holdForGlobalPolicy(ctx monitorContext.Context, app *v1beta1.Application, rev *v1beta1.ApplicationRevision, err error) (monitorContext.Context, error) {
	if keepErr := h.holdResourceKeeper(ctx, app, rev); keepErr != nil {
		ctx.Error(keepErr, "Failed to hold the Application at its current revision")
		return ctx, err
	}
	ctx.Info("Holding the Application at its current revision: a mandatory global policy fails", "revision", rev.Name, "error", err)
	h.heldByGlobalPolicy = rev
	app.Status.SetConditions(condition.ErrorCondition(globalPoliciesCondition, err))
	if policyContext := h.recordedPolicyContext(ctx, app); len(policyContext) > 0 {
		ctx = storeAdditionalContextInCtx(ctx, policyContext)
	}
	return ctx, nil
}

// holdResourceKeeper builds the resource keeper for the held revision: from its spec, so its own
// resource policies apply, and, when the app's own generation and publish version find no
// ResourceTracker (after an edit or a publishVersion bump), from the newest tracker's.
func (h *AppHandler) holdResourceKeeper(ctx monitorContext.Context, app *v1beta1.Application, rev *v1beta1.ApplicationRevision) error {
	_, current, history, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, h.Client, app)
	if err != nil {
		return err
	}
	running := app.DeepCopy()
	running.Spec = *rev.Spec.Application.Spec.DeepCopy()
	if current == nil && len(history) > 0 {
		newest := history[len(history)-1]
		running.Generation = newest.Spec.ApplicationGeneration
		if pv, ok := newest.GetAnnotations()[oam.AnnotationPublishVersion]; ok {
			metav1.SetMetaDataAnnotation(&running.ObjectMeta, oam.AnnotationPublishVersion, pv)
		} else {
			delete(running.Annotations, oam.AnnotationPublishVersion)
		}
	}
	keeper, err := resourcekeeper.NewResourceKeeper(ctx, h.Client, running)
	if err != nil {
		return err
	}
	h.resourceKeeper = keeper
	return nil
}

// recordedPolicyContext returns the policy context last applied to app, from its policy
// observability ConfigMap. A missing or unreadable ConfigMap records none.
func (h *AppHandler) recordedPolicyContext(ctx monitorContext.Context, app *v1beta1.Application) map[string]interface{} {
	cm := &corev1.ConfigMap{}
	if err := h.Client.Get(ctx.GetContext(), client.ObjectKey{Name: policyConfigMapName(app.Namespace, app.Name), Namespace: app.Namespace}, cm); err != nil {
		return nil
	}
	var metadata struct {
		Context map[string]interface{} `json:"context"`
	}
	if json.Unmarshal([]byte(cm.Data["metadata"]), &metadata) != nil {
		return nil
	}
	return metadata.Context
}
