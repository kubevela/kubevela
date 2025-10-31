/*Copyright 2021 The KubeVela Authors.

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
	"encoding/json"
	"strings"
	"time"

	"cuelang.org/go/cue"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/slices"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/executor"
	"github.com/kubevela/workflow/pkg/generator"
	providertypes "github.com/kubevela/workflow/pkg/providers/types"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application/assemble"
	ctrlutil "github.com/oam-dev/kubevela/pkg/controller/utils"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
	"github.com/oam-dev/kubevela/pkg/workflow/template"
)

var (
	// DisableResourceApplyDoubleCheck optimize applyComponentFunc by disable post resource existing check after dispatch
	DisableResourceApplyDoubleCheck = false
)

// GenerateApplicationSteps generate application steps.
// nolint:gocyclo
func (h *AppHandler) GenerateApplicationSteps(ctx monitorContext.Context,
	app *v1beta1.Application,
	appParser *appfile.Parser,
	af *appfile.Appfile) (*wfTypes.WorkflowInstance, []wfTypes.TaskRunner, error) {

	appRev := h.currentAppRev
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("generate-app-steps").Observe(time.Since(t).Seconds())
	}()

	appLabels := map[string]string{
		oam.LabelAppName:      app.Name,
		oam.LabelAppNamespace: app.Namespace,
	}
	pCtx := velaprocess.NewContext(generateContextDataFromApp(app, appRev.Name))
	ctxWithRuntimeParams := oamprovidertypes.WithRuntimeParams(ctx.GetContext(), oamprovidertypes.RuntimeParams{
		ComponentApply:       h.applyComponentFunc(appParser, af),
		ComponentRender:      h.renderComponentFunc(appParser, af),
		ComponentHealthCheck: h.checkComponentHealth(appParser, af),
		WorkloadRender: func(ctx context.Context, comp common.ApplicationComponent) (*appfile.Component, error) {
			return appParser.ParseComponentFromRevisionAndClient(ctx, comp, appRev)
		},
		App:       app,
		AppLabels: appLabels,
		Appfile:   af,
		KubeHandlers: &providertypes.KubeHandlers{
			Apply:  h.Dispatch,
			Delete: h.Delete,
		},
		ConfigFactory: config.NewConfigFactoryWithDispatcher(h.Client, func(ctx context.Context, resources []*unstructured.Unstructured, applyOptions []apply.ApplyOption) error {
			for _, res := range resources {
				res.SetLabels(oamutil.MergeMapOverrideWithDst(res.GetLabels(), appLabels))
			}
			return h.resourceKeeper.Dispatch(ctx, resources, applyOptions)
		}),
		KubeClient: h.Client,
	})
	ctx.SetContext(ctxWithRuntimeParams)
	instance := generateWorkflowInstance(af, app)
	executor.InitializeWorkflowInstance(instance)
	runners, err := generator.GenerateRunners(ctx, instance, wfTypes.StepGeneratorOptions{
		Compiler:       providers.DefaultCompiler.Get(),
		ProcessCtx:     pCtx,
		TemplateLoader: template.NewWorkflowStepTemplateRevisionLoader(appRev, h.Client.RESTMapper()),
		StepConvertor: map[string]func(step workflowv1alpha1.WorkflowStep) (workflowv1alpha1.WorkflowStep, error){
			wfTypes.WorkflowStepTypeApplyComponent: func(lstep workflowv1alpha1.WorkflowStep) (workflowv1alpha1.WorkflowStep, error) {
				copierStep := lstep.DeepCopy()
				if err := convertStepProperties(copierStep, app); err != nil {
					return lstep, errors.WithMessage(err, "convert [apply-component]")
				}
				copierStep.Type = wfTypes.WorkflowStepTypeBuiltinApplyComponent
				return *copierStep, nil
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return instance, runners, nil
}

// CheckWorkflowRestart check if application workflow need restart and return the desired
// rev to be set in status
// 1. If workflow status is empty, it means no previous running record, the
// workflow will restart (cold start)
// 2. If workflow status is not empty, and publishVersion is set, the desired
// rev will be the publishVersion
// 3. If workflow status is not empty, the desired rev will be the
// ApplicationRevision name. For backward compatibility, the legacy style
// <rev>:<hash> will be recognized and reduced into <rev>
func (h *AppHandler) CheckWorkflowRestart(ctx monitorContext.Context, app *v1beta1.Application) {
	desiredRev, currentRev := h.currentAppRev.Name, ""
	if app.Status.Workflow != nil {
		currentRev = app.Status.Workflow.AppRevision
	}
	if metav1.HasAnnotation(app.ObjectMeta, oam.AnnotationPublishVersion) {
		desiredRev = app.GetAnnotations()[oam.AnnotationPublishVersion]
	} else { // nolint
		// backward compatibility
		// legacy versions use <rev>:<hash> as currentRev, extract <rev>
		if idx := strings.LastIndexAny(currentRev, ":"); idx >= 0 {
			currentRev = currentRev[:idx]
		}
	}
	if currentRev != "" && desiredRev == currentRev {
		return
	}
	// record in revision
	if h.latestAppRev != nil && h.latestAppRev.Status.Workflow == nil && app.Status.Workflow != nil {
		app.Status.Workflow.Terminated = true
		app.Status.Workflow.Finished = true
		if app.Status.Workflow.EndTime.IsZero() {
			app.Status.Workflow.EndTime = metav1.Now()
		}
		h.UpdateApplicationRevisionStatus(ctx, h.latestAppRev, app.Status.Workflow)
	}

	// clean recorded resources info.
	app.Status.Services = nil
	app.Status.AppliedResources = nil

	// clean conditions after render
	var reservedConditions []condition.Condition
	for i, cond := range app.Status.Conditions {
		condTpy, err := common.ParseApplicationConditionType(string(cond.Type))
		if err == nil {
			if condTpy <= common.RenderCondition {
				reservedConditions = append(reservedConditions, app.Status.Conditions[i])
			}
		}
	}
	app.Status.Conditions = reservedConditions
	app.Status.Workflow = &common.WorkflowStatus{
		AppRevision: desiredRev,
	}
}

func generateWorkflowInstance(af *appfile.Appfile, app *v1beta1.Application) *wfTypes.WorkflowInstance {
	instance := &wfTypes.WorkflowInstance{
		WorkflowMeta: wfTypes.WorkflowMeta{
			Name:        af.Name,
			Namespace:   af.Namespace,
			Annotations: app.Annotations,
			Labels:      app.Labels,
			UID:         app.UID,
			ChildOwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1beta1.SchemeGroupVersion.String(),
					Kind:       v1beta1.ApplicationKind,
					Name:       app.Name,
					UID:        app.GetUID(),
					Controller: ptr.To(true),
				},
			},
		},
		Debug: af.Debug,
		Steps: af.WorkflowSteps,
		Mode:  af.WorkflowMode,
	}
	status := app.Status.Workflow
	instance.Status = workflowv1alpha1.WorkflowRunStatus{
		Mode:           *af.WorkflowMode,
		Phase:          status.Phase,
		Message:        status.Message,
		Suspend:        status.Suspend,
		SuspendState:   status.SuspendState,
		Terminated:     status.Terminated,
		Finished:       status.Finished,
		ContextBackend: status.ContextBackend,
		Steps:          status.Steps,
		StartTime:      status.StartTime,
		EndTime:        status.EndTime,
	}
	switch app.Status.Phase {
	case common.ApplicationRunning:
		instance.Status.Phase = workflowv1alpha1.WorkflowStateSucceeded
	case common.ApplicationWorkflowSuspending:
		instance.Status.Phase = workflowv1alpha1.WorkflowStateSuspending
	case common.ApplicationWorkflowTerminated:
		instance.Status.Phase = workflowv1alpha1.WorkflowStateTerminated
	default:
		instance.Status.Phase = workflowv1alpha1.WorkflowStateExecuting
	}
	return instance
}

func convertStepProperties(step *workflowv1alpha1.WorkflowStep, app *v1beta1.Application) error {
	o := struct {
		Component string `json:"component"`
		Cluster   string `json:"cluster"`
		Namespace string `json:"namespace"`
	}{}
	js, err := common.RawExtensionPointer{RawExtension: step.Properties}.MarshalJSON()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(js, &o); err != nil {
		return err
	}

	var componentNames []string
	for _, c := range app.Spec.Components {
		componentNames = append(componentNames, c.Name)
	}

	for _, c := range app.Spec.Components {
		if c.Name == o.Component {
			if dcName, ok := checkDependsOnValidComponent(c.DependsOn, componentNames); !ok {
				return errors.Errorf("component %s not found, which is depended by %s", dcName, c.Name)
			}
			step.Inputs = append(step.Inputs, c.Inputs...)
			for index := range step.Inputs {
				parameterKey := strings.TrimSpace(step.Inputs[index].ParameterKey)
				if parameterKey != "" && !strings.HasPrefix(parameterKey, "properties") && !strings.HasPrefix(parameterKey, "traits[") {
					parameterKey = "properties." + parameterKey
				}
				if parameterKey != "" {
					parameterKey = "value." + parameterKey
				}
				step.Inputs[index].ParameterKey = parameterKey
			}
			step.Outputs = append(step.Outputs, c.Outputs...)
			step.DependsOn = append(step.DependsOn, c.DependsOn...)
			c.Inputs = nil
			c.Outputs = nil
			c.DependsOn = nil
			stepProperties := map[string]interface{}{
				"value":   c,
				"cluster": o.Cluster,
			}
			if o.Namespace != "" {
				stepProperties["namespace"] = o.Namespace
			}
			step.Properties = oamutil.Object2RawExtension(stepProperties)
			return nil
		}
	}
	return errors.Errorf("component %s not found", o.Component)
}

func checkDependsOnValidComponent(dependsOnComponentNames, allComponentNames []string) (string, bool) {
	// does not depend on other components
	if dependsOnComponentNames == nil {
		return "", true
	}
	for _, dc := range dependsOnComponentNames {
		if !slices.Contains(allComponentNames, dc) {
			return dc, false
		}
	}
	return "", true
}

func (h *AppHandler) renderComponentFunc(appParser *appfile.Parser, af *appfile.Appfile) oamprovidertypes.ComponentRender {
	return func(baseCtx context.Context, comp common.ApplicationComponent, patcher *cue.Value, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
		ctx := multicluster.ContextWithClusterName(baseCtx, clusterName)

		_, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, patcher, af)
		if err != nil {
			return nil, nil, err
		}
		return renderComponentsAndTraits(manifest, h.currentAppRev, clusterName, overrideNamespace)
	}
}

func (h *AppHandler) checkComponentHealth(appParser *appfile.Parser, af *appfile.Appfile) oamprovidertypes.ComponentHealthCheck {
	return func(baseCtx context.Context, comp common.ApplicationComponent, patcher *cue.Value, clusterName string, overrideNamespace string) (oamprovidertypes.ComponentHealthStatus, *common.ApplicationComponentStatus, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
		ctx := multicluster.ContextWithClusterName(baseCtx, clusterName)
		ctx = contextWithComponentNamespace(ctx, overrideNamespace)
		ctx = contextWithReplicaKey(ctx, comp.ReplicaKey)

		wl, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, patcher, af)
		if err != nil {
			return oamprovidertypes.ComponentUnhealthy, nil, nil, nil, err
		}
		wl.Ctx.SetCtx(auth.ContextWithUserInfo(ctx, h.app))

		readyWorkload, readyTraits, err := renderComponentsAndTraits(manifest, h.currentAppRev, clusterName, overrideNamespace)
		if err != nil {
			return oamprovidertypes.ComponentUnhealthy, nil, nil, nil, err
		}
		checkSkipApplyWorkload(wl)

		dispatchResources := readyTraits
		if !wl.SkipApplyWorkload {
			dispatchResources = append([]*unstructured.Unstructured{readyWorkload}, readyTraits...)
		}
		if !h.resourceKeeper.ContainsResources(dispatchResources) {
			return oamprovidertypes.ComponentUnhealthy, nil, nil, nil, err
		}
		status, output, outputs, isHealth, err := h.collectHealthStatus(auth.ContextWithUserInfo(ctx, h.app), wl, overrideNamespace, false)
		if err != nil {
			return oamprovidertypes.ComponentUnhealthy, nil, nil, nil, err
		}

		// Check health status considering PostDispatch traits
		if !isHealth {
			// Component or non-PostDispatch traits are unhealthy
			return oamprovidertypes.ComponentUnhealthy, status, output, outputs, err
		}

		// Component and non-PostDispatch traits are healthy
		// Now check if there are any PostDispatch traits and their status
		hasPostDispatchTraits := len(manifest.DeferredTraits) > 0
		hasUnhealthyPostDispatch := false
		hasPendingPostDispatch := false

		if status != nil && len(status.Traits) > 0 {
			for _, trait := range status.Traits {
				if trait.Stage == "PostDispatch" {
					state := trait.GetEffectiveState()
					switch state {
					case common.StatePending, common.StateWaiting:
						hasPendingPostDispatch = true
					case common.StateDispatched:
						if !trait.Healthy {
							hasUnhealthyPostDispatch = true
						}
						// If healthy and dispatched, this is good - no flags set
					}
				}
			}
		} else if hasPostDispatchTraits {
			// We have DeferredTraits but no PostDispatch traits in status yet
			// They haven't been rendered/applied yet
			hasPendingPostDispatch = true
		}

		// Determine final health status
		if hasUnhealthyPostDispatch {
			// PostDispatch traits exist but are unhealthy
			return oamprovidertypes.ComponentUnhealthy, status, output, outputs, nil
		} else if hasPendingPostDispatch {
			// Component is ready for PostDispatch but they're still pending
			return oamprovidertypes.ComponentDispatchHealthy, status, output, outputs, nil
		} else {
			// Everything is healthy (including PostDispatch if they exist)
			return oamprovidertypes.ComponentHealthy, status, output, outputs, nil
		}
	}
}

func (h *AppHandler) applyComponentFunc(appParser *appfile.Parser, af *appfile.Appfile) oamprovidertypes.ComponentApply {
	return func(baseCtx context.Context, comp common.ApplicationComponent, patcher *cue.Value, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		t := time.Now()
		appRev := h.currentAppRev
		defer func() { metrics.ApplyComponentTimeHistogram.WithLabelValues("-").Observe(time.Since(t).Seconds()) }()

		ctx := multicluster.ContextWithClusterName(baseCtx, clusterName)
		ctx = contextWithComponentNamespace(ctx, overrideNamespace)
		ctx = contextWithReplicaKey(ctx, comp.ReplicaKey)

		wl, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, patcher, af)
		if err != nil {
			return nil, nil, false, err
		}
		wl.Ctx.SetCtx(auth.ContextWithUserInfo(ctx, h.app))

		readyWorkload, readyTraits, err := renderComponentsAndTraits(manifest, appRev, clusterName, overrideNamespace)
		if err != nil {
			return nil, nil, false, err
		}
		checkSkipApplyWorkload(wl)

		isHealth := true
		if utilfeature.DefaultMutableFeatureGate.Enabled(features.MultiStageComponentApply) {
			manifestDispatchers, err := h.generateDispatcher(appRev, readyWorkload, readyTraits, manifest, overrideNamespace, af.AppAnnotations)
			if err != nil {
				return nil, nil, false, errors.WithMessage(err, "generateDispatcher")
			}

			for _, dispatcher := range manifestDispatchers {
				if isHealth, err := dispatcher.run(ctx, wl, manifest, appRev, clusterName); !isHealth || err != nil {
					return nil, nil, false, err
				}
			}
		} else {
			dispatchResources := readyTraits
			if !wl.SkipApplyWorkload {
				dispatchResources = append([]*unstructured.Unstructured{readyWorkload}, readyTraits...)
			}

			if err := h.Dispatch(ctx, h.Client, clusterName, common.WorkflowResourceCreator, dispatchResources...); err != nil {
				return nil, nil, false, errors.WithMessage(err, "Dispatch")
			}
			_, _, _, isHealth, err = h.collectHealthStatus(ctx, wl, overrideNamespace, false)
			if err != nil {
				return nil, nil, false, errors.WithMessage(err, "CollectHealthStatus")
			}
		}

		if DisableResourceApplyDoubleCheck {
			return readyWorkload, readyTraits, isHealth, nil
		}
		workload, traits, err := getComponentResources(auth.ContextWithUserInfo(ctx, h.app), manifest, wl.SkipApplyWorkload, h.Client)
		return workload, traits, isHealth, err
	}
}

// redirectTraitToLocalIfNeed will override cluster field to be local for traits which are control plane only
func redirectTraitToLocalIfNeed(appRev *v1beta1.ApplicationRevision, readyTraits []*unstructured.Unstructured) []*unstructured.Unstructured {
	traits := readyTraits
	for index, readyTrait := range readyTraits {
		for _, trait := range appRev.Spec.TraitDefinitions {
			if trait.Spec.ControlPlaneOnly && trait.Name == readyTrait.GetLabels()[oam.TraitTypeLabel] {
				oam.SetCluster(traits[index], multicluster.ClusterLocalName)
				traits[index].SetNamespace(appRev.GetNamespace())
				break
			}
		}
	}
	return traits
}

func (h *AppHandler) prepareWorkloadAndManifests(ctx context.Context,
	appParser *appfile.Parser,
	comp common.ApplicationComponent,
	patcher *cue.Value,
	af *appfile.Appfile) (*appfile.Component, *types.ComponentManifest, error) {
	wl, err := appParser.ParseComponentFromRevisionAndClient(ctx, comp, h.currentAppRev)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "ParseWorkload")
	}

	wl.Patch = patcher

	// Check if MultiStageComponentApply is enabled and separate PostDispatch traits
	var deferredTraits []interface{}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.MultiStageComponentApply) {
		immediateTraits, deferredTr := h.separateTraitsByStage(wl.Traits, af.AppAnnotations)
		wl.Traits = immediateTraits
		deferredTraits = deferredTr
	}

	manifest, err := af.GenerateComponentManifest(wl, func(ctxData *velaprocess.ContextData) {
		if ns := componentNamespaceFromContext(ctx); ns != "" {
			ctxData.Namespace = ns
		}
		if rk := replicaKeyFromContext(ctx); rk != "" {
			ctxData.ReplicaKey = rk
		}
		ctxData.Cluster = pkgmulticluster.Local
		if cluster, ok := pkgmulticluster.ClusterFrom(ctx); ok && cluster != "" {
			ctxData.Cluster = cluster
		}
		// cluster info are secrets stored in the control plane cluster
		ctxData.ClusterVersion = multicluster.GetVersionInfoFromObject(pkgmulticluster.WithCluster(ctx, types.ClusterLocalName), h.Client, ctxData.Cluster)
		ctxData.CompRevision, _ = ctrlutil.ComputeSpecHash(comp)
	})
	if err != nil {
		return nil, nil, errors.WithMessage(err, "GenerateComponentManifest")
	}
	if err := af.SetOAMContract(manifest); err != nil {
		return nil, nil, errors.WithMessage(err, "SetOAMContract")
	}

	// Attach deferred traits to manifest if any
	if len(deferredTraits) > 0 {
		manifest.DeferredTraits = deferredTraits
	}

	return wl, manifest, nil
}

func renderComponentsAndTraits(manifest *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	readyWorkload, readyTraits, err := assemble.PrepareBeforeApply(manifest, appRev, DisableAllComponentRevision)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "assemble resources before apply fail")
	}
	if clusterName != "" {
		oam.SetClusterIfEmpty(readyWorkload, clusterName)
		for _, readyTrait := range readyTraits {
			oam.SetClusterIfEmpty(readyTrait, clusterName)
		}
	}
	if overrideNamespace != "" {
		readyWorkload.SetNamespace(overrideNamespace)
		for _, readyTrait := range readyTraits {
			readyTrait.SetNamespace(overrideNamespace)
		}
	}
	readyTraits = redirectTraitToLocalIfNeed(appRev, readyTraits)
	return readyWorkload, readyTraits, nil
}

func checkSkipApplyWorkload(comp *appfile.Component) {
	for _, trait := range comp.Traits {
		if trait.FullTemplate.TraitDefinition.Spec.ManageWorkload {
			comp.SkipApplyWorkload = true
			break
		}
	}
}

func getComponentResources(ctx context.Context, manifest *types.ComponentManifest, skipStandardWorkload bool, cli client.Client) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	var (
		workload *unstructured.Unstructured
		traits   []*unstructured.Unstructured
	)
	if !skipStandardWorkload {
		v := manifest.ComponentOutput.DeepCopy()
		if err := cli.Get(ctx, client.ObjectKeyFromObject(manifest.ComponentOutput), v); err != nil {
			return nil, nil, err
		}
		workload = v
	}

	for _, trait := range manifest.ComponentOutputsAndTraits {
		v := trait.DeepCopy()
		remoteCtx := multicluster.ContextWithClusterName(ctx, oam.GetCluster(v))
		if err := cli.Get(remoteCtx, client.ObjectKeyFromObject(trait), v); err != nil {
			return workload, nil, err
		}
		traits = append(traits, v)
	}
	return workload, traits, nil
}

// generateContextDataFromApp builds the process context for workflow (non-component) execution.
func generateContextDataFromApp(app *v1beta1.Application, appRev string) velaprocess.ContextData {
	data := velaprocess.ContextData{
		Namespace:       app.Namespace,
		AppName:         app.Name,
		CompName:        app.Name,
		AppRevisionName: appRev,
	}
	if app.Annotations != nil {
		data.WorkflowName = app.Annotations[oam.AnnotationWorkflowName]
		data.PublishVersion = app.Annotations[oam.AnnotationPublishVersion]
	}
	// pass labels and annotations to workflow context
	if len(app.Labels) > 0 {
		data.AppLabels = app.Labels
	}
	if len(app.Annotations) > 0 {
		data.AppAnnotations = app.Annotations
	}
	return data
}

// separateTraitsByStage splits traits by dispatch stage
func (h *AppHandler) separateTraitsByStage(traits []*appfile.Trait, annotations map[string]string) ([]*appfile.Trait, []interface{}) {
	var immediateTraits []*appfile.Trait
	var deferredTraits []interface{}

	for _, tr := range traits {
		// Check if this trait is PostDispatch
		stage, err := getTraitDispatchStage(h.Client, tr.Name, h.currentAppRev, annotations)
		if err == nil && stage == PostDispatch {
			// Store this trait for later rendering
			deferredTraits = append(deferredTraits, tr)
		} else {
			// Render this trait immediately
			immediateTraits = append(immediateTraits, tr)
		}
	}

	return immediateTraits, deferredTraits
}

// renderPostDispatchTraits renders deferred traits with status
func (h *AppHandler) renderPostDispatchTraits(_ context.Context, comp *appfile.Component,
	wlOutputStatus map[string]interface{}, wlOutputsStatus map[string]interface{}, deferredTraits []interface{}, appRev *v1beta1.ApplicationRevision, overrideNamespace string) ([]*unstructured.Unstructured, error) {

	var renderedTraits []*unstructured.Unstructured

	// Create Appfile for proper namespace resolution
	namespace := h.app.Namespace
	if overrideNamespace != "" {
		namespace = overrideNamespace
	}
	af := &appfile.Appfile{
		Name:            h.app.Name,
		Namespace:       namespace,
		AppRevisionName: appRev.Name,
	}

	for _, dt := range deferredTraits {
		trait, ok := dt.(*appfile.Trait)
		if !ok {
			klog.Warningf("Expected *appfile.Trait but got %T", dt)
			continue
		}

		ctxData := appfile.GenerateContextDataFromAppFile(af, comp.Name)

		if overrideNamespace != "" {
			ctxData.Namespace = overrideNamespace
		}

		pCtx := appfile.NewBasicContext(ctxData, comp.Params)

		// Inject the component workload status - available as `context.output.status` in CUE
		if wlOutputStatus != nil {
			outputData := map[string]interface{}{
				"status": wlOutputStatus,
			}
			pCtx.PushData("output", outputData)
		}

		// Inject outputs status - available as `context.outputs.<name>` in CUE
		if len(wlOutputsStatus) > 0 {
			pCtx.PushData("outputs", wlOutputsStatus)
		}

		pCtx.PushData(velaprocess.ContextComponentType, comp.Type)

		if err := trait.EvalContext(pCtx); err != nil {
			return nil, errors.Wrapf(err, "failed to evaluate PostDispatch trait %s with status", trait.Name)
		}

		_, assists := pCtx.Output()

		for _, assist := range assists {
			tr, err := assist.Ins.Unstructured()
			if err != nil {
				klog.Warningf("Failed to get unstructured for assist %s: %v", assist.Name, err)
				continue
			}

			// Set namespace using same logic as Appfile.setNamespace
			// Skip namespace resources
			gvk := tr.GetObjectKind().GroupVersionKind()
			if gvk.Kind != "Namespace" && tr.GetNamespace() == "" {
				tr.SetNamespace(af.Namespace)
			}

			labels := map[string]string{
				oam.LabelAppName:      h.app.Name,
				oam.LabelAppNamespace: h.app.Namespace,
				oam.LabelAppComponent: comp.Name,
				oam.TraitTypeLabel:    trait.Name,
			}
			if assist.Name != "" {
				labels[oam.TraitResource] = assist.Name
			}
			oamutil.AddLabels(tr, labels)

			renderedTraits = append(renderedTraits, tr)
		}
	}

	return renderedTraits, nil
}
