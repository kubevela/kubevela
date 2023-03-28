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

package workloads

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

type statefulSetController struct {
	workloadController
	targetNamespacedName types.NamespacedName
}

// add the parent controller to the owner of the StatefulSet, and initialize the size
// before kicking start the update and start from every pod in the old version
func (c *statefulSetController) claimStatefulSet(ctx context.Context, statefulSet *apps.StatefulSet) (bool, error) {
	if controller := metav1.GetControllerOf(statefulSet); controller != nil &&
		(controller.Kind == v1alpha1.RolloutKind && controller.APIVersion == v1alpha1.SchemeGroupVersion.String()) {
		// it's already there
		return true, nil
	}

	statefulSetPatch := client.MergeFrom(statefulSet.DeepCopy())

	// add the parent controller to the owner of the StatefulSet
	ref := metav1.NewControllerRef(c.parentController, c.parentController.GetObjectKind().GroupVersionKind())
	statefulSet.SetOwnerReferences(append(statefulSet.GetOwnerReferences(), *ref))

	// patch the StatefulSet
	if err := c.client.Patch(ctx, statefulSet, statefulSetPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the start the StatefulSet update", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, err
	}
	return false, nil
}

// scale the StatefulSet
func (c *statefulSetController) scaleStatefulSet(ctx context.Context, statefulSet *apps.StatefulSet, size int32) error {
	statefulSetPatch := client.MergeFrom(statefulSet.DeepCopy())
	statefulSet.Spec.Replicas = pointer.Int32(size)

	// patch the StatefulSet
	if err := c.client.Patch(ctx, statefulSet, statefulSetPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning(event.Reason(fmt.Sprintf(
			"Failed to update the StatefulSet %s to the correct target %d", statefulSet.GetName(), size)), err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return err
	}

	klog.InfoS("Submitted upgrade quest for StatefulSet", "StatefulSet",
		statefulSet.GetName(), "target replica size", size, "batch", c.rolloutStatus.CurrentBatch)
	return nil
}

func (c *statefulSetController) setPartition(ctx context.Context, statefulSet *apps.StatefulSet, partition int32) error {
	statefulSetPatch := client.MergeFrom(statefulSet.DeepCopy())
	statefulSet.Spec.UpdateStrategy.RollingUpdate.Partition = pointer.Int32(partition)

	// patch the StatefulSet
	if err := c.client.Patch(ctx, statefulSet, statefulSetPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning(event.Reason(fmt.Sprintf(
			"Failed to update the partition of StatefulSet %s to the correct target %d", statefulSet.GetName(), partition)), err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return err
	}

	klog.InfoS("Submitted upgrade quest for StatefulSet", "StatefulSet",
		statefulSet.GetName(), "target partition", partition, "batch", c.rolloutStatus.CurrentBatch)
	return nil
}

// remove the parent controller from the StatefulSet's owner list
func (c *statefulSetController) releaseStatefulSet(ctx context.Context, statefulSet *apps.StatefulSet) (bool, error) {
	statefulSetPatch := client.MergeFrom(statefulSet.DeepCopy())

	var newOwnerList []metav1.OwnerReference
	found := false
	for _, owner := range statefulSet.GetOwnerReferences() {
		if owner.Kind == v1alpha1.RolloutKind && owner.APIVersion == v1alpha1.SchemeGroupVersion.String() &&
			owner.Controller != nil && *owner.Controller {
			found = true
			continue
		}
		newOwnerList = append(newOwnerList, owner)
	}
	if !found {
		klog.InfoS("the StatefulSet is already released", "StatefulSet", statefulSet.Name)
		return true, nil
	}
	statefulSet.SetOwnerReferences(newOwnerList)

	// patch the StatefulSet
	if err := c.client.Patch(ctx, statefulSet, statefulSetPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the release the StatefulSet", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, err
	}
	return false, nil
}
