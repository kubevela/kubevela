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

package apply

import (
	"context"
	"fmt"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LabelRenderHash is the label that record the hash value of the rendering resource.
	LabelRenderHash = "oam.dev/render-hash"
)

// Applicator applies new state to an object or create it if not exist.
// It uses the same mechanism as `kubectl apply`, that is, for each resource being applied,
// computing a three-way diff merge in client side based on its current state, modified stated,
// and last-applied-state which is tracked through an specific annotation.
// If the resource doesn't exist before, Apply will create it.
type Applicator interface {
	Apply(context.Context, client.Object, ...ApplyOption) error
}

type applyAction struct {
	skipUpdate bool
}

// ApplyOption is called before applying state to the object.
// ApplyOption is still called even if the object does NOT exist.
// If the object does not exist, `existing` will be assigned as `nil`.
// nolint: golint
type ApplyOption func(act *applyAction, existing, desired client.Object) error

// NewAPIApplicator creates an Applicator that applies state to an
// object or creates the object if not exist.
func NewAPIApplicator(c client.Client) *APIApplicator {
	return &APIApplicator{
		creator: creatorFn(createOrGetExisting),
		patcher: patcherFn(threeWayMergePatch),
		c:       c,
	}
}

type creator interface {
	createOrGetExisting(context.Context, *applyAction, client.Client, client.Object, ...ApplyOption) (client.Object, error)
}

type creatorFn func(context.Context, *applyAction, client.Client, client.Object, ...ApplyOption) (client.Object, error)

func (fn creatorFn) createOrGetExisting(ctx context.Context, act *applyAction, c client.Client, o client.Object, ao ...ApplyOption) (client.Object, error) {
	return fn(ctx, act, c, o, ao...)
}

type patcher interface {
	patch(c, m client.Object) (client.Patch, error)
}

type patcherFn func(c, m client.Object) (client.Patch, error)

func (fn patcherFn) patch(c, m client.Object) (client.Patch, error) {
	return fn(c, m)
}

// APIApplicator implements Applicator
type APIApplicator struct {
	creator
	patcher
	c client.Client
}

// loggingApply will record a log with desired object applied
func loggingApply(msg string, desired client.Object) {
	d, ok := desired.(metav1.Object)
	if !ok {
		klog.InfoS(msg, "resource", desired.GetObjectKind().GroupVersionKind().String())
		return
	}
	klog.InfoS(msg, "name", d.GetName(), "resource", desired.GetObjectKind().GroupVersionKind().String())
}

// Apply applies new state to an object or create it if not exist
func (a *APIApplicator) Apply(ctx context.Context, desired client.Object, ao ...ApplyOption) error {
	_, err := generateRenderHash(desired)
	if err != nil {
		return err
	}
	applyAct := new(applyAction)
	existing, err := a.createOrGetExisting(ctx, applyAct, a.c, desired, ao...)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}

	// the object already exists, apply new state
	if err := executeApplyOptions(applyAct, existing, desired, ao); err != nil {
		return err
	}

	if applyAct.skipUpdate {
		loggingApply("skip update", desired)
		return nil
	}

	loggingApply("patching object", desired)
	patch, err := a.patcher.patch(existing, desired)
	if err != nil {
		return errors.Wrap(err, "cannot calculate patch by computing a three way diff")
	}
	return errors.Wrapf(a.c.Patch(ctx, desired, patch), "cannot patch object")
}

func generateRenderHash(desired client.Object) (string, error) {
	if desired == nil {
		return "", nil
	}
	desiredHash, err := utils.ComputeSpecHash(desired)
	if err != nil {
		return "", errors.Wrap(err, "compute desired hash")
	}
	util.AddLabels(desired, map[string]string{
		LabelRenderHash: desiredHash,
	})
	return desiredHash, nil
}

func getRenderHash(existing client.Object) string {
	labels := existing.GetLabels()
	if labels == nil {
		return ""
	}
	return labels[LabelRenderHash]
}

// createOrGetExisting will create the object if it does not exist
// or get and return the existing object
func createOrGetExisting(ctx context.Context, act *applyAction, c client.Client, desired client.Object, ao ...ApplyOption) (client.Object, error) {
	var create = func() (client.Object, error) {
		// execute ApplyOptions even the object doesn't exist
		if err := executeApplyOptions(act, nil, desired, ao); err != nil {
			return nil, err
		}
		if err := addLastAppliedConfigAnnotation(desired); err != nil {
			return nil, err
		}
		loggingApply("creating object", desired)
		return nil, errors.Wrap(c.Create(ctx, desired), "cannot create object")
	}

	// allow to create object with only generateName
	if desired.GetName() == "" && desired.GetGenerateName() != "" {
		return create()
	}

	existing := &unstructured.Unstructured{}
	existing.GetObjectKind().SetGroupVersionKind(desired.GetObjectKind().GroupVersionKind())
	err := c.Get(ctx, types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if kerrors.IsNotFound(err) {
		return create()
	}
	if err != nil {
		return nil, errors.Wrap(err, "cannot get object")
	}
	return existing, nil
}

func executeApplyOptions(act *applyAction, existing, desired client.Object, aos []ApplyOption) error {
	// if existing is nil, it means the object is going to be created.
	// ApplyOption function should handle this situation carefully by itself.
	for _, fn := range aos {
		if err := fn(act, existing, desired); err != nil {
			return errors.Wrap(err, "cannot apply ApplyOption")
		}
	}
	return nil
}

// NotUpdateRenderHashEqual if the render hash of new object equal to the old hash, should not apply.
func NotUpdateRenderHashEqual() ApplyOption {
	return func(act *applyAction, existing, desired client.Object) error {
		if existing == nil || desired == nil {
			return nil
		}
		newSt, ok := desired.(*unstructured.Unstructured)
		if !ok {
			return nil
		}
		oldSt := existing.(*unstructured.Unstructured)
		if !ok {
			return nil
		}
		if getRenderHash(existing) == getRenderHash(desired) {
			*newSt = *oldSt
			act.skipUpdate = true
		}
		return nil
	}
}

// MustBeControllableBy requires that the new object is controllable by an
// object with the supplied UID. An object is controllable if its controller
// reference includes the supplied UID.
func MustBeControllableBy(u types.UID) ApplyOption {
	return func(_ *applyAction, existing, _ client.Object) error {
		if existing == nil {
			return nil
		}
		c := metav1.GetControllerOf(existing.(metav1.Object))
		if c == nil {
			return nil
		}
		if c.UID != u {
			return errors.Errorf("existing object is not controlled by UID %q", u)
		}
		return nil
	}
}

// MustBeControlledByApp requires that the new object is controllable by versioned resourcetracker
func MustBeControlledByApp(app *v1beta1.Application) ApplyOption {
	return func(_ *applyAction, existing, _ client.Object) error {
		if existing == nil {
			return nil
		}
		labels := existing.GetLabels()
		if labels == nil {
			return nil
		}
		if appName, exists := labels[oam.LabelAppName]; exists && appName != app.Name {
			return fmt.Errorf("existing object is managed by other application %s", appName)
		}
		ns := app.Namespace
		if ns == "" {
			ns = metav1.NamespaceDefault
		}
		if appNs, exists := labels[oam.LabelAppNamespace]; exists && appNs != ns {
			return fmt.Errorf("existing object is managed by other application %s/%s", appNs, labels[oam.LabelAppName])
		}
		return nil
	}
}

// MakeCustomApplyOption let user can generate applyOption that restrict change apply action.
func MakeCustomApplyOption(f func(existing, desired client.Object) error) ApplyOption {
	return func(act *applyAction, existing, desired client.Object) error {
		return f(existing, desired)
	}
}
