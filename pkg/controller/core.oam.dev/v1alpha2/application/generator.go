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
	"os"
	"strings"
	"time"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	configprovider "github.com/oam-dev/kubevela/pkg/config/provider"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/utils/apply"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/executor"
	"github.com/kubevela/workflow/pkg/generator"
	"github.com/kubevela/workflow/pkg/providers"
	"github.com/kubevela/workflow/pkg/providers/kube"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/assemble"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	"github.com/oam-dev/kubevela/pkg/stdlib"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	multiclusterProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/multicluster"
	oamProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
	terraformProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/terraform"
	"github.com/oam-dev/kubevela/pkg/workflow/template"
)

func init() {
	if err := stdlib.SetupBuiltinImports(); err != nil {
		klog.ErrorS(err, "Unable to set up builtin imports on package initialization")
		os.Exit(1)
	}
}

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
	handlerProviders := providers.NewProviders()
	kube.Install(handlerProviders, h.r.Client, appLabels, &kube.Handlers{
		Apply:  h.Dispatch,
		Delete: h.Delete,
	})
	configprovider.Install(handlerProviders, h.r.Client, func(ctx context.Context, resources []*unstructured.Unstructured, applyOptions []apply.ApplyOption) error {
		for _, res := range resources {
			res.SetLabels(util.MergeMapOverrideWithDst(res.GetLabels(), appLabels))
		}
		return h.resourceKeeper.Dispatch(ctx, resources, applyOptions)
	})
	oamProvider.Install(handlerProviders, app, af, h.r.Client, h.applyComponentFunc(
		appParser, appRev, af), h.renderComponentFunc(appParser, appRev, af))
	pCtx := velaprocess.NewContext(generateContextDataFromApp(app, appRev.Name))
	renderer := func(ctx context.Context, comp common.ApplicationComponent) (*appfile.Workload, error) {
		return appParser.ParseWorkloadFromRevisionAndClient(ctx, comp, appRev)
	}
	multiclusterProvider.Install(handlerProviders, h.r.Client, app, af,
		h.applyComponentFunc(appParser, appRev, af),
		h.checkComponentHealth(appParser, appRev, af),
		renderer)
	terraformProvider.Install(handlerProviders, app, renderer)
	query.Install(handlerProviders, h.r.Client, nil)

	instance := generateWorkflowInstance(af, app)
	executor.InitializeWorkflowInstance(instance)
	runners, err := generator.GenerateRunners(ctx, instance, wfTypes.StepGeneratorOptions{
		Providers:       handlerProviders,
		PackageDiscover: h.r.pd,
		ProcessCtx:      pCtx,
		TemplateLoader:  template.NewWorkflowStepTemplateRevisionLoader(appRev, h.r.dm),
		Client:          h.r.Client,
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
					Controller: pointer.Bool(true),
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
			step.Properties = util.Object2RawExtension(stepProperties)
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
		if !utils.StringsContain(allComponentNames, dc) {
			return dc, false
		}
	}
	return "", true
}

func (h *AppHandler) renderComponentFunc(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentRender {
	return func(baseCtx context.Context, comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
		ctx := multicluster.ContextWithClusterName(baseCtx, clusterName)

		_, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return nil, nil, err
		}
		return renderComponentsAndTraits(h.r.Client, manifest, appRev, clusterName, overrideNamespace, env)
	}
}

func (h *AppHandler) checkComponentHealth(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentHealthCheck {
	return func(baseCtx context.Context, comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (bool, *unstructured.Unstructured, []*unstructured.Unstructured, error) {
		ctx := multicluster.ContextWithClusterName(baseCtx, clusterName)
		ctx = contextWithComponentNamespace(ctx, overrideNamespace)
		ctx = contextWithReplicaKey(ctx, comp.ReplicaKey)

		wl, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return false, nil, nil, err
		}
		wl.Ctx.SetCtx(auth.ContextWithUserInfo(ctx, h.app))

		readyWorkload, readyTraits, err := renderComponentsAndTraits(h.r.Client, manifest, appRev, clusterName, overrideNamespace, env)
		if err != nil {
			return false, nil, nil, err
		}
		checkSkipApplyWorkload(wl)

		dispatchResources := readyTraits
		if !wl.SkipApplyWorkload {
			dispatchResources = append([]*unstructured.Unstructured{readyWorkload}, readyTraits...)
		}
		if !h.resourceKeeper.ContainsResources(dispatchResources) {
			return false, nil, nil, err
		}

		_, output, outputs, isHealth, err := h.collectHealthStatus(auth.ContextWithUserInfo(ctx, h.app), wl, appRev, overrideNamespace, false)
		if err != nil {
			return false, nil, nil, err
		}

		return isHealth, output, outputs, err
	}
}

func (h *AppHandler) applyComponentFunc(appParser *appfile.Parser, appRev *v1beta1.ApplicationRevision, af *appfile.Appfile) oamProvider.ComponentApply {
	return func(baseCtx context.Context, comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		t := time.Now()
		defer func() { metrics.ApplyComponentTimeHistogram.WithLabelValues("-").Observe(time.Since(t).Seconds()) }()

		ctx := multicluster.ContextWithClusterName(baseCtx, clusterName)
		ctx = contextWithComponentNamespace(ctx, overrideNamespace)
		ctx = contextWithReplicaKey(ctx, comp.ReplicaKey)
		ctx = envbinding.ContextWithEnvName(ctx, env)

		wl, manifest, err := h.prepareWorkloadAndManifests(ctx, appParser, comp, appRev, patcher, af)
		if err != nil {
			return nil, nil, false, err
		}
		if len(manifest.PackagedWorkloadResources) != 0 {
			if err := h.Dispatch(ctx, clusterName, common.WorkflowResourceCreator, manifest.PackagedWorkloadResources...); err != nil {
				return nil, nil, false, errors.WithMessage(err, "cannot dispatch packaged workload resources")
			}
		}
		wl.Ctx.SetCtx(auth.ContextWithUserInfo(ctx, h.app))

		readyWorkload, readyTraits, err := renderComponentsAndTraits(h.r.Client, manifest, appRev, clusterName, overrideNamespace, env)
		if err != nil {
			return nil, nil, false, err
		}
		checkSkipApplyWorkload(wl)

		isHealth := true
		if utilfeature.DefaultMutableFeatureGate.Enabled(features.MultiStageComponentApply) {
			manifestDispatchers, err := h.generateDispatcher(appRev, readyWorkload, readyTraits, overrideNamespace)
			if err != nil {
				return nil, nil, false, errors.WithMessage(err, "generateDispatcher")
			}

			for _, dispatcher := range manifestDispatchers {
				if isHealth, err := dispatcher.run(ctx, wl, appRev, clusterName); !isHealth || err != nil {
					return nil, nil, false, err
				}
			}
		} else {
			dispatchResources := readyTraits
			if !wl.SkipApplyWorkload {
				dispatchResources = append([]*unstructured.Unstructured{readyWorkload}, readyTraits...)
			}

			if err := h.Dispatch(ctx, clusterName, common.WorkflowResourceCreator, dispatchResources...); err != nil {
				return nil, nil, false, errors.WithMessage(err, "Dispatch")
			}
			_, _, _, isHealth, err = h.collectHealthStatus(ctx, wl, appRev, overrideNamespace, false)
			if err != nil {
				return nil, nil, false, errors.WithMessage(err, "CollectHealthStatus")
			}
		}

		if DisableResourceApplyDoubleCheck {
			return readyWorkload, readyTraits, isHealth, nil
		}
		workload, traits, err := getComponentResources(auth.ContextWithUserInfo(ctx, h.app), manifest, wl.SkipApplyWorkload, h.r.Client)
		return workload, traits, isHealth, err
	}
}

// overrideTraits will override cluster field to be local for traits which are control plane only
func overrideTraits(appRev *v1beta1.ApplicationRevision, readyTraits []*unstructured.Unstructured) []*unstructured.Unstructured {
	traits := readyTraits
	for index, readyTrait := range readyTraits {
		for _, trait := range appRev.Spec.TraitDefinitions {
			if trait.Spec.ControlPlaneOnly && trait.Name == readyTrait.GetLabels()[oam.TraitTypeLabel] {
				oam.SetCluster(traits[index], "local")
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
	appRev *v1beta1.ApplicationRevision,
	patcher *value.Value,
	af *appfile.Appfile) (*appfile.Workload, *types.ComponentManifest, error) {
	wl, err := appParser.ParseWorkloadFromRevisionAndClient(ctx, comp, appRev)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "ParseWorkload")
	}
	wl.Patch = patcher
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
		ctxData.ClusterVersion = multicluster.GetVersionInfoFromObject(pkgmulticluster.WithCluster(ctx, types.ClusterLocalName), h.r.Client, ctxData.Cluster)
	})
	if err != nil {
		return nil, nil, errors.WithMessage(err, "GenerateComponentManifest")
	}
	if err := af.SetOAMContract(manifest); err != nil {
		return nil, nil, errors.WithMessage(err, "SetOAMContract")
	}
	if err := h.HandleComponentsRevision(contextWithComponent(ctx, &comp), []*types.ComponentManifest{manifest}); err != nil {
		return nil, nil, errors.WithMessage(err, "HandleComponentsRevision")
	}
	return wl, manifest, nil
}

func renderComponentsAndTraits(client client.Client, manifest *types.ComponentManifest, appRev *v1beta1.ApplicationRevision, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	readyWorkload, readyTraits, err := assemble.PrepareBeforeApply(manifest, appRev, []assemble.WorkloadOption{assemble.DiscoveryHelmBasedWorkload(context.TODO(), client)})
	if err != nil {
		return nil, nil, errors.WithMessage(err, "assemble resources before apply fail")
	}
	if clusterName != "" {
		oam.SetCluster(readyWorkload, clusterName)
		for _, readyTrait := range readyTraits {
			oam.SetCluster(readyTrait, clusterName)
		}
	}
	if overrideNamespace != "" {
		readyWorkload.SetNamespace(overrideNamespace)
		for _, readyTrait := range readyTraits {
			readyTrait.SetNamespace(overrideNamespace)
		}
	}
	readyTraits = overrideTraits(appRev, readyTraits)
	if env != "" {
		meta.AddLabels(readyWorkload, map[string]string{oam.LabelAppEnv: env})
		for _, readyTrait := range readyTraits {
			meta.AddLabels(readyTrait, map[string]string{oam.LabelAppEnv: env})
		}
	}
	return readyWorkload, readyTraits, nil
}

func checkSkipApplyWorkload(wl *appfile.Workload) {
	for _, trait := range wl.Traits {
		if trait.FullTemplate.TraitDefinition.Spec.ManageWorkload {
			wl.SkipApplyWorkload = true
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
		v := manifest.StandardWorkload.DeepCopy()
		if err := cli.Get(ctx, client.ObjectKeyFromObject(manifest.StandardWorkload), v); err != nil {
			return nil, nil, err
		}
		workload = v
	}

	for _, trait := range manifest.Traits {
		v := trait.DeepCopy()
		remoteCtx := multicluster.ContextWithClusterName(ctx, oam.GetCluster(v))
		if err := cli.Get(remoteCtx, client.ObjectKeyFromObject(trait), v); err != nil {
			return workload, nil, err
		}
		traits = append(traits, v)
	}
	return workload, traits, nil
}

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
	return data
}
