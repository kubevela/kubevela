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
	"sync"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// ResourceKeeper handler for dispatching and deleting resources
type ResourceKeeper interface {
	Dispatch(context.Context, []*unstructured.Unstructured, []apply.ApplyOption, ...DispatchOption) error
	Delete(context.Context, []*unstructured.Unstructured, ...DeleteOption) error
	GarbageCollect(context.Context, ...GCOption) (bool, []v1beta1.ManagedResource, error)
	StateKeep(context.Context) error
	ContainsResources([]*unstructured.Unstructured) bool

	DispatchComponentRevision(context.Context, *appsv1.ControllerRevision) error
	DeleteComponentRevision(context.Context, *appsv1.ControllerRevision) error
}

type resourceKeeper struct {
	client.Client
	app *v1beta1.Application
	mu  sync.Mutex

	applicator  apply.Applicator
	_rootRT     *v1beta1.ResourceTracker
	_currentRT  *v1beta1.ResourceTracker
	_historyRTs []*v1beta1.ResourceTracker
	_crRT       *v1beta1.ResourceTracker

	applyOncePolicy      *v1alpha1.ApplyOncePolicySpec
	garbageCollectPolicy *v1alpha1.GarbageCollectPolicySpec
	sharedResourcePolicy *v1alpha1.SharedResourcePolicySpec
	takeOverPolicy       *v1alpha1.TakeOverPolicySpec
	readOnlyPolicy       *v1alpha1.ReadOnlyPolicySpec

	cache *resourceCache
}

func (h *resourceKeeper) getRootRT(ctx context.Context) (rootRT *v1beta1.ResourceTracker, err error) {
	if h._rootRT == nil {
		if h._rootRT, err = resourcetracker.CreateRootResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, h.app); err != nil {
			return nil, err
		}
	}
	return h._rootRT, nil
}

func (h *resourceKeeper) getCurrentRT(ctx context.Context) (currentRT *v1beta1.ResourceTracker, err error) {
	if h._currentRT == nil {
		if h._currentRT, err = resourcetracker.CreateCurrentResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, h.app); err != nil {
			return nil, err
		}
	}
	return h._currentRT, nil
}

func (h *resourceKeeper) getComponentRevisionRT(ctx context.Context) (crRT *v1beta1.ResourceTracker, err error) {
	if h._crRT == nil {
		if h._crRT, err = resourcetracker.CreateComponentRevisionResourceTracker(multicluster.ContextInLocalCluster(ctx), h.Client, h.app); err != nil {
			return nil, err
		}
	}
	return h._crRT, nil
}

func (h *resourceKeeper) parseApplicationResourcePolicy() (err error) {
	if h.applyOncePolicy, err = policy.ParsePolicy[v1alpha1.ApplyOncePolicySpec](h.app); err != nil {
		return errors.Wrapf(err, "failed to parse apply-once policy")
	}
	if h.applyOncePolicy == nil && metav1.HasLabel(h.app.ObjectMeta, oam.LabelAddonName) {
		h.applyOncePolicy = &v1alpha1.ApplyOncePolicySpec{Enable: true}
	}
	if h.garbageCollectPolicy, err = policy.ParsePolicy[v1alpha1.GarbageCollectPolicySpec](h.app); err != nil {
		return errors.Wrapf(err, "failed to parse garbage-collect policy")
	}
	if h.sharedResourcePolicy, err = policy.ParsePolicy[v1alpha1.SharedResourcePolicySpec](h.app); err != nil {
		return errors.Wrapf(err, "failed to parse shared-resource policy")
	}
	if h.takeOverPolicy, err = policy.ParsePolicy[v1alpha1.TakeOverPolicySpec](h.app); err != nil {
		return errors.Wrapf(err, "failed to parse take-over policy")
	}
	if h.readOnlyPolicy, err = policy.ParsePolicy[v1alpha1.ReadOnlyPolicySpec](h.app); err != nil {
		return errors.Wrapf(err, "failed to parse read-only policy")
	}
	return nil
}

func (h *resourceKeeper) loadResourceTrackers(ctx context.Context) (err error) {
	h._rootRT, h._currentRT, h._historyRTs, h._crRT, err = resourcetracker.ListApplicationResourceTrackers(multicluster.ContextInLocalCluster(ctx), h.Client, h.app)
	return err
}

// NewResourceKeeper create a handler for dispatching and deleting resources
func NewResourceKeeper(ctx context.Context, cli client.Client, app *v1beta1.Application) (_ ResourceKeeper, err error) {
	h := &resourceKeeper{
		Client:     cli,
		app:        app,
		applicator: apply.NewAPIApplicator(cli),
		cache:      newResourceCache(cli, app),
	}
	if err = h.loadResourceTrackers(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to load resourcetrackers")
	}
	if err = h.parseApplicationResourcePolicy(); err != nil {
		return nil, errors.Wrapf(err, "failed to parse resource policy")
	}
	return h, nil
}
