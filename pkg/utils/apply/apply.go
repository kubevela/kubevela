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
	"reflect"

	"github.com/kubevela/pkg/util/builder"
	utilhash "github.com/kubevela/pkg/util/hash"
	"github.com/kubevela/pkg/util/jsonutil"
	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/k8s/apply"
	velapatch "github.com/kubevela/pkg/util/k8s/patch"
	"github.com/kubevela/pkg/util/slices"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Apply applies new state to an object or create it if not exist
func Apply(ctx context.Context, c client.Client, desired client.Object, opts ...Option) error {
	if desired == nil {
		return nil
	}
	options := &Options{updateAnno: trimLastAppliedConfigurationForSpecialResources(desired)}
	builder.ApplyTo(options, slices.Map(opts, func(o Option) builder.Option[Options] { return o })...)
	return apply.Apply(ctx, c, desired, options)
}

var _ apply.Options = &Options{}
var _ apply.PatchActionProvider = &Options{}
var _ apply.PreApplyHook = &Options{}
var _ apply.PreCreateHook = &Options{}
var _ apply.PreUpdateHook = &Options{}

// Option for Apply
type Option builder.Option[Options]

// Options options for Apply
type Options struct {
	controlledBy *v1beta1.Application
	sharedBy     *v1beta1.Application
	dryRun       bool

	updateAnno               bool
	notUpdateRenderHashEqual bool
	readOnly                 bool
	takeOver                 bool

	updateStrategy v1alpha1.ResourceUpdateStrategy
}

// GetPatchAction implement apply.PatchActionProvider
func (in *Options) GetPatchAction() velapatch.PatchAction {
	return velapatch.PatchAction{
		UpdateAnno:            in.updateAnno,
		AnnoLastAppliedConfig: oam.AnnotationLastAppliedConfig,
		AnnoLastAppliedTime:   oam.AnnotationLastAppliedTime,
	}
}

// PreUpdate implement apply.PreUpdateHook
func (in *Options) PreUpdate(existing, desired client.Object) error {
	sharer := k8s.GetAnnotation(existing, oam.AnnotationAppSharedBy)
	if in.controlledBy != nil {
		appKey, controlledBy := GetAppKey(in.controlledBy), GetControlledBy(existing)
		if controlledBy == "" && !utilfeature.DefaultMutableFeatureGate.Enabled(features.LegacyResourceOwnerValidation) && existing.GetResourceVersion() != "" && !in.takeOver {
			return fmt.Errorf("%s %s/%s exists but not managed by any application now", existing.GetObjectKind().GroupVersionKind().Kind, existing.GetNamespace(), existing.GetName())
		}
		if controlledBy != "" && controlledBy != appKey && (in.sharedBy == nil || len(sharer) == 0) {
			return fmt.Errorf("existing object %s %s/%s is managed by other application %s", existing.GetObjectKind().GroupVersionKind().Kind, existing.GetNamespace(), existing.GetName(), controlledBy)
		}
	}
	if in.sharedBy != nil {
		if err := jsonutil.CopyInto(existing, desired); err != nil {
			return fmt.Errorf("failed to copy exisiting object %s %s/%s to desired for sharing: %w", existing.GetObjectKind().GroupVersionKind().Kind, existing.GetNamespace(), existing.GetName(), err)
		}
		if in.controlledBy != nil && len(sharer) == 0 {
			_ = k8s.AddLabel(desired, oam.LabelAppName, in.controlledBy.Name)
			_ = k8s.AddLabel(desired, oam.LabelAppNamespace, in.controlledBy.Namespace)
		}
		_ = k8s.AddAnnotation(desired, oam.AnnotationAppSharedBy, AddSharer(sharer, in.sharedBy))
	}
	return nil
}

// PreCreate implement apply.PreCreateHook
func (in *Options) PreCreate(desired client.Object) error {
	if in.readOnly {
		return fmt.Errorf("%s (%s) is marked as read-only but does not exist. You should check the existence of the resource or remove the read-only policy", desired.GetObjectKind().GroupVersionKind().Kind, desired.GetName())
	}
	if in.controlledBy != nil {
		_ = k8s.AddLabel(desired, oam.LabelAppName, in.controlledBy.Name)
		_ = k8s.AddLabel(desired, oam.LabelAppNamespace, in.controlledBy.Namespace)
	}
	if in.sharedBy != nil {
		_ = k8s.AddAnnotation(desired, oam.AnnotationAppSharedBy, AddSharer("", in.sharedBy))
	}
	return nil
}

// PreApply implement apply.PreApplyHook
func (in *Options) PreApply(desired client.Object) error {
	desiredHash, err := utilhash.ComputeHash(desired)
	if err != nil {
		return fmt.Errorf("compute hash error: %w", err)
	}
	_ = k8s.AddLabel(desired, oam.LabelRenderHash, desiredHash)
	if in.updateAnno {
		if err = velapatch.AddLastAppliedConfiguration(desired, oam.AnnotationLastAppliedConfig, oam.AnnotationLastAppliedTime); err != nil {
			return err
		}
	}
	return nil
}

// DryRun implement apply.Option
func (in *Options) DryRun() apply.DryRunOption {
	return apply.DryRunOption(in.dryRun)
}

// GetUpdateStrategy implement apply.Option
func (in *Options) GetUpdateStrategy(existing, desired client.Object) (apply.UpdateStrategy, error) {
	if in.readOnly {
		return apply.Skip, nil
	}
	if in.notUpdateRenderHashEqual && k8s.GetLabel(existing, oam.LabelRenderHash) == k8s.GetLabel(desired, oam.LabelRenderHash) && in.sharedBy == nil {
		return apply.Skip, jsonutil.CopyInto(existing, desired)
	}
	shouldRecreate, err := needRecreate(in.updateStrategy.RecreateFields, existing, desired)
	if err != nil {
		return apply.Skip, fmt.Errorf("failed to evaluate recreateFields: %w", err)
	}
	if shouldRecreate {
		return apply.Recreate, nil
	}
	switch in.updateStrategy.Op {
	case v1alpha1.ResourceUpdateStrategyPatch:
		return apply.Patch, nil
	case v1alpha1.ResourceUpdateStrategyReplace:
		return apply.Replace, nil
	case "":
		if utilfeature.DefaultMutableFeatureGate.Enabled(features.ApplyResourceByReplace) && isUpdatableResource(desired) {
			return apply.Replace, nil
		}
		return apply.Patch, nil
	default:
		return apply.Skip, fmt.Errorf("unrecognizable update strategy op: %s", in.updateStrategy.Op)
	}
}

func needRecreate(recreateFields []string, existing, desired client.Object) (bool, error) {
	if len(recreateFields) == 0 {
		return false, nil
	}
	_existing, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(existing)
	_desired, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(desired)
	flag := false
	for _, field := range recreateFields {
		ve, err := jsonutil.LookupPath(_existing, field)
		if err != nil {
			return false, fmt.Errorf("unable to get path %s from existing object: %w", field, err)
		}
		vd, err := jsonutil.LookupPath(_desired, field)
		if err != nil {
			return false, fmt.Errorf("unable to get path %s from desired object: %w", field, err)
		}
		if !reflect.DeepEqual(ve, vd) {
			flag = true
		}
	}
	return flag, nil
}

// isUpdatableResource check whether the resource is updatable
// Resource like v1.Service cannot unset the spec field (the ip spec is filled by service controller)
func isUpdatableResource(desired client.Object) bool {
	// nolint
	switch desired.GetObjectKind().GroupVersionKind() {
	case corev1.SchemeGroupVersion.WithKind("Service"):
		return false
	}
	return true
}

// trimLastAppliedConfigurationForSpecialResources will filter special object that can reduce the record for "app.oam.dev/last-applied-configuration" annotation.
func trimLastAppliedConfigurationForSpecialResources(desired client.Object) bool {
	if gvk := desired.GetObjectKind().GroupVersionKind(); gvk.Group == "" {
		// group is empty means it's Kubernetes core API, we won't record annotation for Secret and Configmap
		_, ok1 := desired.(*corev1.ConfigMap)
		_, ok2 := desired.(*corev1.Secret)
		_, ok3 := desired.(*apiextensionsv1.CustomResourceDefinition)
		if ok1 || ok2 || ok3 || slices.Contains([]string{"Secret", "ConfigMap", "CustomResourceDefinition"}, gvk.Kind) {
			return false
		}
	}
	lac := k8s.GetAnnotation(desired, oam.AnnotationLastAppliedConfig)
	return lac != "-" && lac != "skip"
}

// GetControlledBy extract the application that controls the current resource
func GetControlledBy(existing client.Object) string {
	appName := k8s.GetLabel(existing, oam.LabelAppName)
	appNs := k8s.GetLabel(existing, oam.LabelAppNamespace)
	if appName == "" || appNs == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", appNs, appName)
}

// GetAppKey construct the key for identifying the application
func GetAppKey(app *v1beta1.Application) string {
	ns := app.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}
	return fmt.Sprintf("%s/%s", ns, app.GetName())
}

// NotUpdateRenderHashEqual if the render hash of new object equal to the old hash, should not apply.
func NotUpdateRenderHashEqual() Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.notUpdateRenderHashEqual = true
	})
}

// ReadOnly skip apply fo the resource
func ReadOnly() Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.readOnly = true
	})
}

// TakeOver allow take over resources without app owner
func TakeOver() Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.takeOver = true
	})
}

// WithUpdateStrategy set the update strategy for the apply operation
func WithUpdateStrategy(strategy v1alpha1.ResourceUpdateStrategy) Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.updateStrategy = strategy
	})
}

// MustBeControlledByApp requires that the new object is controllable by versioned resourcetracker
func MustBeControlledByApp(app *v1beta1.Application) Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.controlledBy = app
	})
}

// DisableUpdateAnnotation disable write last config to annotation
func DisableUpdateAnnotation() Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.updateAnno = false
	})
}

// SharedByApp let the resource be sharable
func SharedByApp(app *v1beta1.Application) Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.sharedBy = app
	})
}

// DryRunAll executing all validation, etc without persisting the change to storage.
func DryRunAll() Option {
	return builder.OptionFn[Options](func(a *Options) {
		a.dryRun = true
	})
}
