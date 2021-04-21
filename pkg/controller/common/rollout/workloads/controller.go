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

	"github.com/crossplane/crossplane-runtime/pkg/event"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// WorkloadController is the interface that all type of cloneSet controller implements
type WorkloadController interface {
	// VerifySpec makes sure that the resources can be upgraded according to the rollout plan
	// it returns if the verification succeeded/failed or should retry
	VerifySpec(ctx context.Context) (bool, error)

	// Initialize make sure that the resource is ready to be upgraded
	// this function is tasked to do any initialization work on the resources
	// it returns if the initialization succeeded/failed or should retry
	Initialize(ctx context.Context) (bool, error)

	// RolloutOneBatchPods tries to upgrade pods in the resources following the rollout plan
	// it will upgrade pods as the rollout plan allows at once
	// it returns if the upgrade actionable items succeeded/failed or should continue
	RolloutOneBatchPods(ctx context.Context) (bool, error)

	// CheckOneBatchPods checks how many pods are ready to serve requests in the current batch
	// it returns whether the number of pods upgraded in this round satisfies the rollout plan
	CheckOneBatchPods(ctx context.Context) (bool, error)

	// FinalizeOneBatch makes sure that the rollout can start the next batch
	// it returns if the finalization of this batch succeeded/failed or should retry
	FinalizeOneBatch(ctx context.Context) (bool, error)

	// Finalize makes sure the resources are in a good final state.
	// It might depend on if the rollout succeeded or not.
	// For example, we may remove the source object to prevent scalar traits to ever work
	// and the finalize rollout web hooks will be called after this call succeeds
	Finalize(ctx context.Context, succeed bool) bool
}

type workloadController struct {
	client           client.Client
	recorder         event.Recorder
	parentController oam.Object

	rolloutSpec   *v1alpha1.RolloutPlan
	rolloutStatus *v1alpha1.RolloutStatus
}

// cloneSetController is the place to hold fields needed for handle Cloneset type of workloads
type cloneSetController struct {
	workloadController
	targetNamespacedName types.NamespacedName
	cloneSet             *kruise.CloneSet
}

// size fetches the Cloneset and returns the replicas (not the actual number of pods)
func (c *cloneSetController) size(ctx context.Context) (int32, error) {
	if c.cloneSet == nil {
		err := c.fetchCloneSet(ctx)
		if err != nil {
			return 0, err
		}
	}
	// default is 1
	if c.cloneSet.Spec.Replicas == nil {
		return 1, nil
	}
	return *c.cloneSet.Spec.Replicas, nil
}

func (c *cloneSetController) fetchCloneSet(ctx context.Context) error {
	// get the cloneSet
	workload := kruise.CloneSet{}
	err := c.client.Get(ctx, c.targetNamespacedName, &workload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			c.recorder.Event(c.parentController, event.Warning("Failed to get the Cloneset", err))
		}
		return err
	}
	c.cloneSet = &workload
	return nil
}
