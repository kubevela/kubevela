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

package applicationconfiguration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	pkgutil "github.com/oam-dev/kubevela/pkg/utils"
)

// ControllerRevisionComponentLabel indicate which component the revision belong to
// This label is to filter revision by client api
const ControllerRevisionComponentLabel = oam.LabelControllerRevisionComponent

// ComponentHandler will watch component change and generate Revision automatically.
type ComponentHandler struct {
	Client                client.Client
	RevisionLimit         int
	CustomRevisionHookURL string
}

// Create implements EventHandler
func (c *ComponentHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	reqs, succeed := c.createControllerRevision(evt.Object, evt.Object)
	if !succeed {
		// No revision created, return
		return
	}
	for _, req := range reqs {
		q.Add(req)
	}
}

// Update implements EventHandler
func (c *ComponentHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	reqs, succeed := c.createControllerRevision(evt.ObjectNew, evt.ObjectNew)
	if !succeed {
		// No revision created, return
		return
	}
	// Note(wonderflow): MetaOld => MetaNew, requeue once is enough
	for _, req := range reqs {
		q.Add(req)
	}
}

// Delete implements EventHandler
func (c *ComponentHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	// controllerRevision will be deleted by ownerReference mechanism
	// so we don't need to delete controllerRevision here.
	// but trigger an event to AppConfig controller, let it know.
	for _, req := range c.getRelatedAppConfig(evt.Object) {
		q.Add(req)
	}
}

// Generic implements EventHandler
func (c *ComponentHandler) Generic(_ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
	// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
	// so we need to do nothing here.
}

func isMatch(appConfigs *v1alpha2.ApplicationConfigurationList, compName string) (bool, types.NamespacedName) {
	for _, app := range appConfigs.Items {
		for _, comp := range app.Spec.Components {
			if comp.ComponentName == compName || utils.ExtractComponentName(comp.RevisionName) == compName {
				return true, types.NamespacedName{Namespace: app.Namespace, Name: app.Name}
			}
		}
	}
	return false, types.NamespacedName{}
}

func (c *ComponentHandler) getRelatedAppConfig(object metav1.Object) []reconcile.Request {
	var appConfigs v1alpha2.ApplicationConfigurationList
	err := c.Client.List(context.Background(), &appConfigs)
	if err != nil {
		klog.Info(fmt.Sprintf("error list all applicationConfigurations %v", err))
		return nil
	}
	var reqs []reconcile.Request
	if match, namespaceName := isMatch(&appConfigs, object.GetName()); match {
		reqs = append(reqs, reconcile.Request{NamespacedName: namespaceName})
	}
	return reqs
}

// IsRevisionDiff check whether there's any different between two component revision
func (c *ComponentHandler) IsRevisionDiff(mt klog.KMetadata, curComp *v1alpha2.Component) (bool, int64) {
	if curComp.Status.LatestRevision == nil {
		return true, 0
	}

	// client in controller-runtime will use informer cache
	// use client will be more efficient
	needNewRevision, err := utils.CompareWithRevision(context.TODO(), c.Client, mt.GetName(), mt.GetNamespace(),
		curComp.Status.LatestRevision.Name, &curComp.Spec)
	// TODO: this might be a bug that we treat all errors getting from k8s as a new revision
	// but the client go event handler doesn't handle an error. We need to see if we can retry this
	if err != nil {
		klog.InfoS(fmt.Sprintf("Failed to compare the component with its latest revision with err = %+v", err),
			"component", mt.GetName(), "latest revision", curComp.Status.LatestRevision.Name)
		return true, curComp.Status.LatestRevision.Revision
	}
	return needNewRevision, curComp.Status.LatestRevision.Revision
}

func (c *ComponentHandler) createControllerRevision(mt metav1.Object, obj client.Object) ([]reconcile.Request, bool) {
	curComp := obj.(*v1alpha2.Component)
	comp := curComp.DeepCopy()
	// No generation changed, will not create revision
	if comp.Generation == comp.Status.ObservedGeneration {
		return nil, false
	}
	diff, curRevision := c.IsRevisionDiff(mt, comp)
	if !diff {
		// No difference, no need to create new revision.
		return nil, false
	}

	reqs := c.getRelatedAppConfig(mt)
	// Hook to custom revision service if exist
	if err := c.customComponentRevisionHook(reqs, comp); err != nil {
		klog.InfoS(fmt.Sprintf("fail to hook from custom revision service(%s) %v", c.CustomRevisionHookURL, err), "componentName", mt.GetName())
		return nil, false
	}

	nextRevision := curRevision + 1
	revisionName := utils.ConstructRevisionName(mt.GetName(), nextRevision)

	if comp.Status.ObservedGeneration != comp.Generation {
		comp.Status.ObservedGeneration = comp.Generation
	}

	comp.Status.LatestRevision = &common.Revision{
		Name:     revisionName,
		Revision: nextRevision,
	}
	compRaw, err := json.Marshal(comp)
	if err != nil {
		klog.InfoS(fmt.Sprintf("json.Marshal failed:  %v", err), "componentName", mt.GetName())
		return nil, false
	}
	// set annotation to component
	revision := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionName,
			Namespace: comp.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.ComponentKind,
					Name:       comp.Name,
					UID:        comp.UID,
					Controller: pointer.Bool(true),
				},
			},
			Labels: map[string]string{
				ControllerRevisionComponentLabel: pkgutil.EscapeResourceNameToLabelValue(comp.Name),
			},
		},
		Revision: nextRevision,
		Data:     runtime.RawExtension{Raw: compRaw},
	}

	// TODO: we should update the status first. otherwise, the subsequent create will all fail if the update fails
	err = c.Client.Create(context.TODO(), revision)
	if err != nil {
		klog.InfoS(fmt.Sprintf("error create controllerRevision %v", err), "componentName", mt.GetName())
		return nil, false
	}

	err = c.UpdateStatus(context.Background(), comp)
	if err != nil {
		klog.InfoS(fmt.Sprintf("update component status latestRevision %s err %v", revisionName, err), "componentName", mt.GetName())
		return nil, false
	}

	klog.InfoS("Create ControllerRevision", "name", revisionName)
	// garbage collect
	if int64(c.RevisionLimit) < nextRevision {
		if err := c.cleanupControllerRevision(comp); err != nil {
			klog.Info(fmt.Sprintf("failed to clean up revisions of Component %v.", err))
		}
	}
	return reqs, true
}

// get sorted controllerRevisions, prepare to delete controllerRevisions
func sortedControllerRevision(appConfigs []v1alpha2.ApplicationConfiguration, revisions []appsv1.ControllerRevision,
	revisionLimit int) (sortedRevisions []appsv1.ControllerRevision, toKill int, liveHashes map[string]bool) {
	liveHashes = make(map[string]bool)
	sortedRevisions = revisions

	// get all revisions used and skipped
	for _, appConfig := range appConfigs {
		for _, component := range appConfig.Spec.Components {
			if component.RevisionName != "" {
				liveHashes[component.RevisionName] = true
			}
		}
	}

	toKeep := revisionLimit + len(liveHashes)
	toKill = len(sortedRevisions) - toKeep
	if toKill <= 0 {
		toKill = 0
		return
	}
	// Clean up old revisions from smallest to highest revision (from oldest to newest)
	sort.Sort(historiesByRevision(sortedRevisions))

	return
}

// clean revisions when over limits
func (c *ComponentHandler) cleanupControllerRevision(curComp *v1alpha2.Component) error {
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			ControllerRevisionComponentLabel: pkgutil.EscapeResourceNameToLabelValue(curComp.Name),
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labels)
	if err != nil {
		return err
	}

	// List and Get Object, controller-runtime will create Informer cache
	// and will get objects from cache
	revisions := &appsv1.ControllerRevisionList{}
	if err := c.Client.List(context.TODO(), revisions, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}

	// Get appConfigs and workloads filter controllerRevision used
	appConfigs := &v1alpha2.ApplicationConfigurationList{}
	if err := c.Client.List(context.Background(), appConfigs); err != nil {
		return err
	}

	// get sorted revisions
	controllerRevisions, toKill, liveHashes := sortedControllerRevision(appConfigs.Items, revisions.Items, c.RevisionLimit)
	for _, revision := range controllerRevisions {
		if toKill <= 0 {
			break
		}
		if hash := revision.GetName(); liveHashes[hash] {
			continue
		}
		// Clean up
		revisionToClean := revision
		if err := c.Client.Delete(context.TODO(), &revisionToClean); err != nil {
			return err
		}
		klog.InfoS("Delete controllerRevision", "name", revision.Name)
		toKill--
	}
	return nil
}

// UpdateStatus updates v1alpha2.Component's Status with retry.RetryOnConflict
func (c *ComponentHandler) UpdateStatus(ctx context.Context, comp *v1alpha2.Component, opts ...client.SubResourceUpdateOption) error {
	status := comp.DeepCopy().Status
	return retry.RetryOnConflict(retry.DefaultBackoff, func() (err error) {
		if err = c.Client.Get(ctx, types.NamespacedName{Namespace: comp.Namespace, Name: comp.Name}, comp); err != nil {
			return
		}
		comp.Status = status
		return c.Client.Status().Update(ctx, comp, opts...)
	})
}

// historiesByRevision sort controllerRevision by revision
type historiesByRevision []appsv1.ControllerRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Revision < h[j].Revision
}
