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
	"reflect"
	"time"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/util/singleton"
	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ValidateWorkflow validates the Application workflow
func (h *ValidatingHandler) ValidateWorkflow(_ context.Context, app *v1beta1.Application) field.ErrorList {
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
	appParser := appfile.NewApplicationParser(cli)

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
	return componentErrs
}

// checkDefinitionPermission checks if user has permission to access a definition in either system namespace or app namespace
func (h *ValidatingHandler) checkDefinitionPermission(ctx context.Context, req admission.Request, resource, definitionType, appNamespace string) (bool, error) {
	// Check permission in vela-system namespace first since most definitions are there
	// This optimizes for the common case and reduces API calls
	systemNsSar := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User:   req.UserInfo.Username,
			Groups: req.UserInfo.Groups,
			ResourceAttributes: &authv1.ResourceAttributes{
				Verb:      "get",
				Group:     "core.oam.dev",
				Version:   "v1beta1",
				Resource:  resource,
				Namespace: oam.SystemDefinitionNamespace,
				Name:      definitionType,
			},
		},
	}

	if err := h.Client.Create(ctx, systemNsSar); err != nil {
		return false, fmt.Errorf("failed to check %s permission in system namespace: %w", resource, err)
	}

	if systemNsSar.Status.Allowed {
		// User has permission in system namespace
		// Verify the definition actually exists in vela-system
		if exists, err := h.definitionExistsInNamespace(ctx, resource, definitionType, oam.SystemDefinitionNamespace); err != nil {
			klog.Errorf("Failed to check if %s %q exists in vela-system: %v", resource, definitionType, err)
			// On error checking existence, propagate the error so caller can distinguish system failures from permission denials
			return false, err
		} else if !exists {
			klog.V(4).Infof("%s %q does not exist in vela-system, checking app namespace", resource, definitionType)
			// Definition doesn't exist in vela-system, fall through to check app namespace
		} else {
			// Definition exists in vela-system and user has permission
			return true, nil
		}
	}

	// If not in system namespace and app namespace is different, check app namespace
	if appNamespace != oam.SystemDefinitionNamespace {
		appNsSar := &authv1.SubjectAccessReview{
			Spec: authv1.SubjectAccessReviewSpec{
				User:   req.UserInfo.Username,
				Groups: req.UserInfo.Groups,
				ResourceAttributes: &authv1.ResourceAttributes{
					Verb:      "get",
					Group:     "core.oam.dev",
					Version:   "v1beta1",
					Resource:  resource,
					Namespace: appNamespace,
					Name:      definitionType,
				},
			},
		}

		if err := h.Client.Create(ctx, appNsSar); err != nil {
			return false, fmt.Errorf("failed to check %s permission in namespace %s: %w", resource, appNamespace, err)
		}

		if appNsSar.Status.Allowed {
			// User has permission in app namespace
			// But we need to verify the definition actually exists in the app namespace
			// to prevent users with wildcard permissions from using definitions that only exist in vela-system
			if exists, err := h.definitionExistsInNamespace(ctx, resource, definitionType, appNamespace); err != nil {
				klog.V(4).Infof("Failed to check if %s %q exists in namespace %q: %v", resource, definitionType, appNamespace, err)
				// On error checking existence, propagate the error
				return false, err
			} else if !exists {
				klog.V(4).Infof("%s %q does not exist in namespace %q, denying access", resource, definitionType, appNamespace)
				return false, nil
			}
			// Definition exists and user has permission
			return true, nil
		}
	}

	// User doesn't have permission in either namespace
	return false, nil
}

// definitionExistsInNamespace checks if a definition actually exists in the specified namespace
func (h *ValidatingHandler) definitionExistsInNamespace(ctx context.Context, resource, name, namespace string) (bool, error) {
	// Determine the object type based on the resource
	var obj client.Object
	switch resource {
	case "componentdefinitions":
		obj = &v1beta1.ComponentDefinition{}
	case "traitdefinitions":
		obj = &v1beta1.TraitDefinition{}
	case "policydefinitions":
		obj = &v1beta1.PolicyDefinition{}
	case "workflowstepdefinitions":
		obj = &v1beta1.WorkflowStepDefinition{}
	default:
		return false, fmt.Errorf("unknown resource type: %s", resource)
	}

	// Try to get the definition from the namespace
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := h.Client.Get(ctx, key, obj); err != nil {
		if !errors.IsNotFound(err) {
			// Handle other errors than not found
			return false, err
		}
		// Definition not found
		return false, nil
	}

	// Definition exists
	return true, nil
}

// workflowStepLocation represents the location of a workflow step
type workflowStepLocation struct {
	StepIndex    int
	SubStepIndex int // -1 if not a sub-step
	IsSubStep    bool
}

// definitionUsage tracks where each definition type is used in the application
type definitionUsage struct {
	componentTypes    map[string][]int
	traitTypes        map[string][][2]int
	policyTypes       map[string][]int
	workflowStepTypes map[string][]workflowStepLocation
}

// collectDefinitionUsage collects all unique definition types and their locations in the application
func collectDefinitionUsage(app *v1beta1.Application) *definitionUsage {
	usage := &definitionUsage{
		componentTypes:    make(map[string][]int),
		traitTypes:        make(map[string][][2]int),
		policyTypes:       make(map[string][]int),
		workflowStepTypes: make(map[string][]workflowStepLocation),
	}

	// Collect component and trait types
	for i, comp := range app.Spec.Components {
		usage.componentTypes[comp.Type] = append(usage.componentTypes[comp.Type], i)

		for j, trait := range comp.Traits {
			usage.traitTypes[trait.Type] = append(usage.traitTypes[trait.Type], [2]int{i, j})
		}
	}

	// Collect policy types
	for i, policy := range app.Spec.Policies {
		usage.policyTypes[policy.Type] = append(usage.policyTypes[policy.Type], i)
	}

	// Collect workflow step types (including sub-steps)
	if app.Spec.Workflow != nil {
		for i, step := range app.Spec.Workflow.Steps {
			location := workflowStepLocation{
				StepIndex:    i,
				SubStepIndex: -1,
				IsSubStep:    false,
			}
			usage.workflowStepTypes[step.Type] = append(usage.workflowStepTypes[step.Type], location)

			// Also check sub-steps
			for j, subStep := range step.SubSteps {
				subLocation := workflowStepLocation{
					StepIndex:    i,
					SubStepIndex: j,
					IsSubStep:    true,
				}
				usage.workflowStepTypes[subStep.Type] = append(usage.workflowStepTypes[subStep.Type], subLocation)
			}
		}
	}

	return usage
}

// processDefinitionPermissionCheck handles the common logic for processing permission check results
func (h *ValidatingHandler) processDefinitionPermissionCheck(
	allowed bool,
	err error,
	req admission.Request,
	definitionKind string,
	definitionType string,
	appNamespace string,
	fieldPaths []*field.Path,
) field.ErrorList {
	var errs field.ErrorList

	if err != nil {
		klog.Errorf("Failed to check %s permission for user %s: %v", definitionKind, req.UserInfo.Username, err)
		for _, fieldPath := range fieldPaths {
			errs = append(errs, field.Forbidden(fieldPath,
				fmt.Sprintf("unable to verify permissions for %s %q: %v", definitionKind, definitionType, err)))
		}
		return errs
	}

	if !allowed {
		klog.Infof("User %q does not have permission to access %s %q in namespace %q or %q",
			req.UserInfo.Username, definitionKind, definitionType, appNamespace, oam.SystemDefinitionNamespace)
		for _, fieldPath := range fieldPaths {
			errs = append(errs, field.Forbidden(fieldPath,
				fmt.Sprintf("user %q cannot get %s %q in namespace %q or %q",
					req.UserInfo.Username, definitionKind, definitionType, appNamespace, oam.SystemDefinitionNamespace)))
		}
	}

	return errs
}

// fieldPathBuilder is a function that builds field paths for a given definition usage
type fieldPathBuilder func(interface{}) []*field.Path

// validateDefinitions is a generic function to validate any definition type
func (h *ValidatingHandler) validateDefinitions(
	ctx context.Context,
	req admission.Request,
	appNamespace string,
	definitionType reflect.Type,
	usageMap interface{},
	buildFieldPaths fieldPathBuilder,
) field.ErrorList {
	var errs field.ErrorList

	// Get the definition info for the given type
	defInfo, ok := v1beta1.DefinitionTypeMap[definitionType]
	if !ok {
		klog.Errorf("Unknown definition type: %v", definitionType)
		return errs
	}

	switch typedMap := usageMap.(type) {
	case map[string][]int:
		for defType, indices := range typedMap {
			allowed, err := h.checkDefinitionPermission(ctx, req, defInfo.GVR.Resource, defType, appNamespace)
			fieldPaths := buildFieldPaths(indices)
			errs = append(errs, h.processDefinitionPermissionCheck(
				allowed, err, req, defInfo.Kind, defType, appNamespace, fieldPaths)...)
		}
	case map[string][][2]int:
		for defType, locations := range typedMap {
			allowed, err := h.checkDefinitionPermission(ctx, req, defInfo.GVR.Resource, defType, appNamespace)
			fieldPaths := buildFieldPaths(locations)
			errs = append(errs, h.processDefinitionPermissionCheck(
				allowed, err, req, defInfo.Kind, defType, appNamespace, fieldPaths)...)
		}
	case map[string][]workflowStepLocation:
		for defType, locations := range typedMap {
			allowed, err := h.checkDefinitionPermission(ctx, req, defInfo.GVR.Resource, defType, appNamespace)
			fieldPaths := buildFieldPaths(locations)
			errs = append(errs, h.processDefinitionPermissionCheck(
				allowed, err, req, defInfo.Kind, defType, appNamespace, fieldPaths)...)
		}
	}

	return errs
}

// ValidateDefinitionPermissions validates that the user has permissions to access all definition types
func (h *ValidatingHandler) ValidateDefinitionPermissions(ctx context.Context, app *v1beta1.Application, req admission.Request) field.ErrorList {
	// Check if definition validation is enabled
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateDefinitionPermissions) {
		return nil
	}

	var errs field.ErrorList
	usage := collectDefinitionUsage(app)

	// Validate ComponentDefinitions
	errs = append(errs, h.validateDefinitions(ctx, req, app.Namespace,
		reflect.TypeOf(v1beta1.ComponentDefinition{}), usage.componentTypes,
		func(indices interface{}) []*field.Path {
			var paths []*field.Path
			for _, idx := range indices.([]int) {
				paths = append(paths, field.NewPath("spec", "components").Index(idx).Child("type"))
			}
			return paths
		})...)

	// Validate TraitDefinitions
	errs = append(errs, h.validateDefinitions(ctx, req, app.Namespace,
		reflect.TypeOf(v1beta1.TraitDefinition{}), usage.traitTypes,
		func(locations interface{}) []*field.Path {
			var paths []*field.Path
			for _, loc := range locations.([][2]int) {
				paths = append(paths,
					field.NewPath("spec", "components").Index(loc[0]).Child("traits").Index(loc[1]).Child("type"))
			}
			return paths
		})...)

	// Validate PolicyDefinitions
	errs = append(errs, h.validateDefinitions(ctx, req, app.Namespace,
		reflect.TypeOf(v1beta1.PolicyDefinition{}), usage.policyTypes,
		func(indices interface{}) []*field.Path {
			var paths []*field.Path
			for _, idx := range indices.([]int) {
				paths = append(paths, field.NewPath("spec", "policies").Index(idx).Child("type"))
			}
			return paths
		})...)

	// Validate WorkflowStepDefinitions
	errs = append(errs, h.validateDefinitions(ctx, req, app.Namespace,
		reflect.TypeOf(v1beta1.WorkflowStepDefinition{}), usage.workflowStepTypes,
		func(locations interface{}) []*field.Path {
			var paths []*field.Path
			for _, loc := range locations.([]workflowStepLocation) {
				paths = append(paths, getWorkflowStepFieldPath(loc))
			}
			return paths
		})...)

	return errs
}

// getWorkflowStepFieldPath constructs the field path for a workflow step or sub-step
func getWorkflowStepFieldPath(loc workflowStepLocation) *field.Path {
	if !loc.IsSubStep {
		// Regular step
		return field.NewPath("spec", "workflow", "steps").Index(loc.StepIndex).Child("type")
	}
	// Sub-step
	return field.NewPath("spec", "workflow", "steps").Index(loc.StepIndex).Child("subSteps").Index(loc.SubStepIndex).Child("type")
}

// ValidateAnnotations validates whether the application has both autoupdate and publish version annotations
func (h *ValidatingHandler) ValidateAnnotations(_ context.Context, app *v1beta1.Application) field.ErrorList {
	var annotationsErrs field.ErrorList

	hasPublishVersion := app.Annotations[oam.AnnotationPublishVersion]
	hasAutoUpdate := app.Annotations[oam.AnnotationAutoUpdate]
	if hasAutoUpdate == "true" && hasPublishVersion != "" {
		annotationsErrs = append(annotationsErrs, field.Invalid(field.NewPath("metadata", "annotations"), app,
			"Application has both autoUpdate and publishVersion annotations. Only one can be present"))
	}
	return annotationsErrs
}

// ValidateCreate validates the Application on creation
func (h *ValidatingHandler) ValidateCreate(ctx context.Context, app *v1beta1.Application, req admission.Request) field.ErrorList {
	var errs field.ErrorList

	errs = append(errs, h.ValidateAnnotations(ctx, app)...)
	errs = append(errs, h.ValidateDefinitionPermissions(ctx, app, req)...)
	errs = append(errs, h.ValidateWorkflow(ctx, app)...)
	errs = append(errs, h.ValidateComponents(ctx, app)...)
	return errs
}

// ValidateUpdate validates the Application on update
func (h *ValidatingHandler) ValidateUpdate(ctx context.Context, newApp, _ *v1beta1.Application, req admission.Request) field.ErrorList {
	// check if the newApp is valid
	errs := h.ValidateCreate(ctx, newApp, req)
	// TODO: add more validating
	return errs
}
