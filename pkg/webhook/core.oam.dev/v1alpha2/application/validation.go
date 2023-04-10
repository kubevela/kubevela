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

package application

import (
	"context"
	"fmt"
	"time"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/util/singleton"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// ValidateWorkflow validates the Application workflow
func (h *ValidatingHandler) ValidateWorkflow(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var errs field.ErrorList
	if app.Spec.Workflow != nil {
		stepName := make(map[string]interface{})
		for _, step := range app.Spec.Workflow.Steps {
			if _, ok := stepName[step.Name]; ok {
				errs = append(errs, field.Invalid(field.NewPath("spec", "workflow", "steps"), step.Name, "duplicated step name"))
			}
			stepName[step.Name] = nil
			if step.Timeout != "" {
				errs = append(errs, h.ValidateTimeout(step.Name, step.Timeout)...)
			}
			for _, sub := range step.SubSteps {
				if _, ok := stepName[sub.Name]; ok {
					errs = append(errs, field.Invalid(field.NewPath("spec", "workflow", "steps", "subSteps"), sub.Name, "duplicated step name"))
				}
				stepName[sub.Name] = nil
				if step.Timeout != "" {
					errs = append(errs, h.ValidateTimeout(step.Name, step.Timeout)...)
				}
			}
		}
	}
	return errs
}

// ValidateTimeout validates the timeout of steps
func (h *ValidatingHandler) ValidateTimeout(name, timeout string) field.ErrorList {
	var errs field.ErrorList
	_, err := time.ParseDuration(timeout)
	if err != nil {
		errs = append(errs, field.Invalid(field.NewPath("spec", "workflow", "steps", "timeout"), name, "invalid timeout, please use the format of timeout like 1s, 1m, 1h or 1d"))
	}
	return errs
}

// appRevBypassCacheClient
type appRevBypassCacheClient struct {
	client.Client
}

// Get retrieve appRev directly from request if sharding enabled
func (in *appRevBypassCacheClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	if _, ok := obj.(*v1beta1.ApplicationRevision); ok && sharding.EnableSharding {
		return singleton.KubeClient.Get().Get(ctx, key, obj)
	}
	return in.Client.Get(ctx, key, obj)
}

// ValidateComponents validates the Application components
func (h *ValidatingHandler) ValidateComponents(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	if sharding.EnableSharding && !utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateComponentWhenSharding) {
		return nil
	}
	var componentErrs field.ErrorList
	// try to generate an app file
	cli := &appRevBypassCacheClient{Client: h.Client}
	appParser := appfile.NewApplicationParser(cli, h.dm, h.pd)

	af, err := appParser.GenerateAppFile(ctx, app)
	if err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("spec"), app, err.Error()))
		// cannot generate appfile, no need to validate further
		return componentErrs
	}
	if i, err := appParser.ValidateComponentNames(app); err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].name", i)), app, err.Error()))
	}
	if err := appParser.ValidateCUESchematicAppfile(af); err != nil {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("schematic"), app, err.Error()))
	}
	if v := app.GetAnnotations()[oam.AnnotationAppRollout]; len(v) != 0 && v != "true" {
		componentErrs = append(componentErrs, field.Invalid(field.NewPath("annotation:app.oam.dev/rollout-template"), app, "the annotation value of rollout-template must be true"))
	}
	componentErrs = append(componentErrs, h.validateExternalRevisionName(ctx, app)...)
	return componentErrs
}

// ValidateCreate validates the Application on creation
func (h *ValidatingHandler) ValidateCreate(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var errs field.ErrorList

	errs = append(errs, h.ValidateWorkflow(ctx, app)...)
	errs = append(errs, h.ValidateComponents(ctx, app)...)
	return errs
}

// ValidateUpdate validates the Application on update
func (h *ValidatingHandler) ValidateUpdate(ctx context.Context, newApp, oldApp *v1beta1.Application) field.ErrorList {
	// check if the newApp is valid
	errs := h.ValidateCreate(ctx, newApp)
	// TODO: add more validating
	return errs
}

func (h *ValidatingHandler) validateExternalRevisionName(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var componentErrs field.ErrorList

	for index, comp := range app.Spec.Components {
		if len(comp.ExternalRevision) == 0 {
			continue
		}

		revisionName := comp.ExternalRevision
		cr := &appsv1.ControllerRevision{}
		if err := h.Client.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: revisionName}, cr); err != nil {
			if !apierrors.IsNotFound(err) {
				componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, err.Error()))
			}
			continue
		}

		labeledControllerComponent := cr.GetLabels()[oam.LabelControllerRevisionComponent]
		if labeledControllerComponent != comp.Name {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, fmt.Sprintf("label:%s for revision:%s should be equal with component name", oam.LabelControllerRevisionComponent, revisionName)))
			continue
		}

		comp, err := util.RawExtension2Component(cr.Data)
		if err != nil {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, "can't covert revision to component"))
			continue
		}
		_, err = util.RawExtension2Unstructured(&comp.Spec.Workload)
		if err != nil {
			componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, "can't extract workload"))
			continue
		}
		if comp.Spec.Helm != nil {
			_, err = util.RawExtension2Unstructured(&comp.Spec.Helm.Release)
			if err != nil {
				componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, "can't extract helmRelease"))
				continue
			}
			_, err = util.RawExtension2Unstructured(&comp.Spec.Helm.Repository)
			if err != nil {
				componentErrs = append(componentErrs, field.Invalid(field.NewPath(fmt.Sprintf("components[%d].externalRevision", index)), app, "can't extract helmRepository"))
				continue
			}
		}
	}
	return componentErrs
}
