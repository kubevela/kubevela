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

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	apps "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// deploymentController is the place to hold fields needed for handle Deployment type of workloads
type deploymentController struct {
	workloadController
	targetNamespacedName types.NamespacedName
}

// add the parent controller to the owner of the deployment, unpause it and initialize the size
// before kicking start the update and start from every pod in the old version
func (c *deploymentController) claimDeployment(ctx context.Context, deploy *apps.Deployment, initSize *int32) (bool, error) {
	if controller := metav1.GetControllerOf(deploy); controller != nil && controller.APIVersion == v1alpha1.SchemeGroupVersion.String() &&
		controller.Kind == v1alpha1.RolloutKind {
		// it's already there
		return true, nil
	}

	deployPatch := client.MergeFrom(deploy.DeepCopy())

	// add the parent controller to the owner of the deployment
	ref := metav1.NewControllerRef(c.parentController, c.parentController.GetObjectKind().GroupVersionKind())
	deploy.SetOwnerReferences(append(deploy.GetOwnerReferences(), *ref))

	deploy.Spec.Paused = false
	if initSize != nil {
		deploy.Spec.Replicas = initSize
	}

	// patch the Deployment
	if err := c.client.Patch(ctx, deploy, deployPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the start the Deployment update", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, err
	}
	return false, nil
}

// scale the deployment
func (c *deploymentController) scaleDeployment(ctx context.Context, deploy *apps.Deployment, size int32) error {
	deployPatch := client.MergeFrom(deploy.DeepCopy())
	deploy.Spec.Replicas = pointer.Int32(size)

	// patch the Deployment
	if err := c.client.Patch(ctx, deploy, deployPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning(event.Reason(fmt.Sprintf(
			"Failed to update the deployment %s to the correct target %d", deploy.GetName(), size)), err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return err
	}

	klog.InfoS("Submitted upgrade quest for deployment", "deployment",
		deploy.GetName(), "target replica size", size, "batch", c.rolloutStatus.CurrentBatch)
	return nil
}

// remove the parent controller from the deployment's owner list
func (c *deploymentController) releaseDeployment(ctx context.Context, deploy *apps.Deployment) (bool, error) {
	deployPatch := client.MergeFrom(deploy.DeepCopy())

	var newOwnerList []metav1.OwnerReference
	found := false
	for _, owner := range deploy.GetOwnerReferences() {
		if owner.Kind == c.parentController.GetObjectKind().GroupVersionKind().Kind &&
			owner.APIVersion == c.parentController.GetObjectKind().GroupVersionKind().GroupVersion().String() &&
			owner.Controller != nil && *owner.Controller {
			found = true
			continue
		}
		newOwnerList = append(newOwnerList, owner)
	}
	if !found {
		klog.InfoS("the deployment is already released", "deploy", deploy.Name)
		return true, nil
	}
	deploy.SetOwnerReferences(newOwnerList)

	// patch the Deployment
	if err := c.client.Patch(ctx, deploy, deployPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Event(c.parentController, event.Warning("Failed to the release the Deployment", err))
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, err
	}
	return false, nil
}
