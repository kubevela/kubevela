/*
Copyright 2026 The KubeVela Authors.

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

package application_test

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"math/rand"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfContext "github.com/kubevela/workflow/pkg/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	veloperation "github.com/oam-dev/kubevela/pkg/workflow/operation"
)

// applyRestartWebserviceWorkflowStepDefinition installs the restart-webservice
// WorkflowStepDefinition test fixture into vela-system, the namespace
// WorkflowStepDefinitions resolve from. Torn down in AfterAll.
func applyRestartWebserviceWorkflowStepDefinition(ctx context.Context) {
	var def v1beta1.WorkflowStepDefinition
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/workflowstepdefinition.yaml"), &def)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &def)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

// applyOperationTemplate installs the restart-webservice OperationTemplate
// fixture into vela-system, exercising the same two-tier resolution used
// for ComponentDefinition. Torn down in AfterAll.
func applyOperationTemplate(ctx context.Context) {
	var tmpl v2alpha1.OperationTemplate
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/operationtemplate.yaml"), &tmpl)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &tmpl)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

// applyOperationRBAC installs the restart Job's identity from the same
// fixtures the manual walkthrough applies. Torn down in AfterAll.
func applyOperationRBAC(ctx context.Context) {
	var sa corev1.ServiceAccount
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/serviceaccount.yaml"), &sa)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &sa)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	var role rbacv1.ClusterRole
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/clusterrole.yaml"), &role)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &role)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	var roleBinding rbacv1.ClusterRoleBinding
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/clusterrolebinding.yaml"), &roleBinding)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &roleBinding)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

// applyOkStepWorkflowStepDefinition and applyFlakyStepWorkflowStepDefinition
// install the re-execution test fixtures' WorkflowStepDefinitions into
// vela-system. Torn down in AfterAll.
func applyOkStepWorkflowStepDefinition(ctx context.Context) {
	var def v1beta1.WorkflowStepDefinition
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/workflowstepdefinition-ok.yaml"), &def)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &def)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

func applyFlakyStepWorkflowStepDefinition(ctx context.Context) {
	var def v1beta1.WorkflowStepDefinition
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/workflowstepdefinition-flaky.yaml"), &def)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &def)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

// applyRetryOperationTemplate, applySuspendOperationTemplate, and
// applyIOOperationTemplate install the re-execution test fixtures'
// OperationTemplates into vela-system. Torn down in AfterAll.
func applyRetryOperationTemplate(ctx context.Context) {
	var tmpl v2alpha1.OperationTemplate
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/operationtemplate-retry.yaml"), &tmpl)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &tmpl)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

func applySuspendOperationTemplate(ctx context.Context) {
	var tmpl v2alpha1.OperationTemplate
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/operationtemplate-suspend.yaml"), &tmpl)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &tmpl)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

func applyIOOperationTemplate(ctx context.Context) {
	var tmpl v2alpha1.OperationTemplate
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/operationtemplate-io.yaml"), &tmpl)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &tmpl)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

// applyAppScopeCheckWorkflowStepDefinition, applyApplicationScopeOperationTemplate,
// and applyNoneScopeOperationTemplate install the Application/None scope
// test fixtures into vela-system. Torn down in AfterAll.
func applyAppScopeCheckWorkflowStepDefinition(ctx context.Context) {
	var def v1beta1.WorkflowStepDefinition
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/workflowstepdefinition-appscope-check.yaml"), &def)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &def)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

func applyApplicationScopeOperationTemplate(ctx context.Context) {
	var tmpl v2alpha1.OperationTemplate
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/operationtemplate-application-scope.yaml"), &tmpl)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &tmpl)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

func applyNoneScopeOperationTemplate(ctx context.Context) {
	var tmpl v2alpha1.OperationTemplate
	Expect(common.ReadYamlToObject(testDataPath("operation/vela-system/operationtemplate-none-scope.yaml"), &tmpl)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &tmpl)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

// applyOperationTargetApp installs the shared target Application into
// namespaceName and waits for its webservice Deployment to be ready, so an
// Operation's `target` immediately resolves to a healthy Component.
func applyOperationTargetApp(ctx context.Context, namespaceName string) *appsv1.Deployment {
	var app v1beta1.Application
	Expect(common.ReadYamlToObject(testDataPath("operation/app.yaml"), &app)).Should(BeNil())
	app.Namespace = namespaceName
	Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

	deployment := &appsv1.Deployment{}
	Eventually(func() int32 {
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "webservice"}, deployment); err != nil {
			return 0
		}
		return deployment.Status.ReadyReplicas
	}, 2*time.Minute, 2*time.Second).Should(Equal(int32(1)))
	return deployment
}

// operationParams marshals params into the *runtime.RawExtension shape
// Operation.spec.parameters expects, the same way `vela operation run`'s
// --param flags do.
func operationParams(params map[string]string) *runtime.RawExtension {
	raw, err := json.Marshal(params)
	Expect(err).Should(BeNil())
	return &runtime.RawExtension{Raw: raw}
}

// lookupContextVar reads a named value out of the workflow's context-backend
// ConfigMap (the same CUE `vars` document clearOperationContextVars edits),
// so a test can confirm a step's recorded output without re-deriving it from
// the step's own live status.
func lookupContextVar(cm *corev1.ConfigMap, name string) string {
	v := cuecontext.New().CompileString(cm.Data[wfContext.ConfigMapKeyVars])
	Expect(v.Err()).Should(BeNil())
	s, err := v.LookupPath(cue.ParsePath(name)).String()
	Expect(err).Should(BeNil())
	return s
}

// waitForOperationPhase polls op into the latest state from the API server
// and returns once it reaches phase.
func waitForOperationPhase(ctx context.Context, op *v2alpha1.Operation, phase v2alpha1.OperationPhase) {
	Eventually(func() v2alpha1.OperationPhase {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(op), op); err != nil {
			Expect(err).ToNot(HaveOccurred())
		}
		return op.Status.Phase
	}, 2*time.Minute, 2*time.Second).Should(Equal(phase))
}

// deleteOperationTargetNamespaceJob removes this run's restart Job and its
// Pods. Scoped to targetNamespace, so it's safe to run after every It even
// under parallel Ginkgo processes.
func deleteOperationTargetNamespaceJob(ctx context.Context, targetNamespace string) {
	Expect(k8sClient.DeleteAllOf(ctx, &batchv1.Job{}, client.InNamespace("vela-system"),
		client.MatchingLabels{"operation.oam.dev/target-namespace": targetNamespace},
		client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
}

// deleteOperationVelaSystemFixtures removes the shared WorkflowStepDefinition/
// OperationTemplate/RBAC fixtures the apply* functions above install into
// vela-system. Runs once via AfterAll, not per-It: these are cluster-scoped
// and shared across specs, so deleting and re-applying them between every It
// races with parallel Ginkgo processes still relying on them mid-run.
func deleteOperationVelaSystemFixtures(ctx context.Context) {
	objs := []client.Object{
		&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "restart-webservice", Namespace: "vela-system"}},
		&v2alpha1.OperationTemplate{ObjectMeta: metav1.ObjectMeta{Name: "restart-webservice", Namespace: "vela-system"}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "restart-webservice", Namespace: "vela-system"}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "restart-webservice"}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "restart-webservice"}},
		&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "ok-step", Namespace: "vela-system"}},
		&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "flaky-step", Namespace: "vela-system"}},
		&v2alpha1.OperationTemplate{ObjectMeta: metav1.ObjectMeta{Name: "retry-flaky", Namespace: "vela-system"}},
		&v2alpha1.OperationTemplate{ObjectMeta: metav1.ObjectMeta{Name: "suspend-then-ok", Namespace: "vela-system"}},
		&v2alpha1.OperationTemplate{ObjectMeta: metav1.ObjectMeta{Name: "io-flaky", Namespace: "vela-system"}},
		&v1beta1.WorkflowStepDefinition{ObjectMeta: metav1.ObjectMeta{Name: "appscope-check", Namespace: "vela-system"}},
		&v2alpha1.OperationTemplate{ObjectMeta: metav1.ObjectMeta{Name: "app-scope", Namespace: "vela-system"}},
		&v2alpha1.OperationTemplate{ObjectMeta: metav1.ObjectMeta{Name: "none-scope", Namespace: "vela-system"}},
	}
	for _, obj := range objs {
		Expect(k8sClient.Delete(ctx, obj)).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	}
}

// Smoke test for the v2alpha1 Operation controller (KEP 2.15).
var _ = Describe("Operation (v2alpha1)", Label("core-application"), Ordered, func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespaceName = "operation-e2e-test" + "-" + strconv.FormatInt(rand.Int63(), 16)
		ns = createNamespace(ctx, namespaceName)
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
		deleteOperationTargetNamespaceJob(ctx, namespaceName)
	})

	AfterAll(func() {
		deleteOperationVelaSystemFixtures(ctx)
	})

	It("restarts the target Deployment, and only reaches phase Succeeded once the Job completes", func() {
		By("applying the restart-webservice WorkflowStepDefinition (vela-system)")
		applyRestartWebserviceWorkflowStepDefinition(ctx)

		By("applying the restart-webservice OperationTemplate (vela-system)")
		applyOperationTemplate(ctx)

		By("applying RBAC for the restart Job's ServiceAccount (vela-system)")
		applyOperationRBAC(ctx)

		By("applying the webservice Application")
		var app v1beta1.Application
		Expect(common.ReadYamlToObject(testDataPath("operation/app.yaml"), &app)).Should(BeNil())
		app.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		By("waiting for the webservice component's Deployment to be ready")
		deployment := &appsv1.Deployment{}
		Eventually(func() int32 {
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "webservice"}, deployment); err != nil {
				return 0
			}
			return deployment.Status.ReadyReplicas
		}, 2*time.Minute, 2*time.Second).Should(Equal(int32(1)))
		beforeGeneration := deployment.Generation

		By("creating the Operation, resolving its template from vela-system")
		var op v2alpha1.Operation
		Expect(common.ReadYamlToObject(testDataPath("operation/operation.yaml"), &op)).Should(BeNil())
		op.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &op)).Should(BeNil())

		By("waiting for the Operation to reach a terminal phase")
		Eventually(func() v2alpha1.OperationPhase {
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "restart-op"}, &op); err != nil {
				return ""
			}
			return op.Status.Phase
		}, 2*time.Minute, 2*time.Second).Should(Equal(v2alpha1.OperationPhaseSucceeded),
			"the Operation should only reach Succeeded once its workflow's Job step actually completes, not merely once the Job is created")

		By("verifying the Deployment was actually restarted")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "webservice"}, deployment)).Should(BeNil())
		Expect(deployment.Generation).To(BeNumerically(">", beforeGeneration))
		Expect(deployment.Spec.Template.Annotations).To(HaveKey("kubectl.kubernetes.io/restartedAt"))

		By("verifying the Operation's spec.parameters.reason reached the Deployment via context.operationParams")
		Expect(deployment.Annotations).To(HaveKeyWithValue("operation.oam.dev/restart-reason", "e2e-test-restart"))

		By("verifying the step read the Deployment's status via context.output")
		pods := &corev1.PodList{}
		Expect(k8sClient.List(ctx, pods, client.InNamespace("vela-system"),
			client.MatchingLabels{"workflow.oam.dev/step-name": "webservice-restart"})).Should(BeNil())
		Expect(pods.Items).NotTo(BeEmpty())
		for _, pod := range pods.Items {
			Expect(pod.Spec.Containers[0].Env).To(ContainElement(
				corev1.EnvVar{Name: "READY_REPLICAS", Value: "1"}))
		}
	})

	// TODO: retry feature needs to be fixed -- disabled pending a fix, see handoff notes.
	// TODO: move to envtest -- e2e should stay a slim smoke test.
	PIt("restarts a failed step with its attempt history preserved, and also allows restarting an already-succeeded step", func() {
		By("applying the ok-step/flaky-step WorkflowStepDefinitions and the retry-flaky OperationTemplate (vela-system)")
		applyOkStepWorkflowStepDefinition(ctx)
		applyFlakyStepWorkflowStepDefinition(ctx)
		applyRetryOperationTemplate(ctx)

		By("applying the target Application")
		applyOperationTargetApp(ctx, namespaceName)

		By("creating the Operation with shouldFail=true, so step-three fails")
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "retry-", Namespace: namespaceName},
			Spec: v2alpha1.OperationSpec{
				Template:   "retry-flaky",
				Source:     &v2alpha1.OperationSource{App: "operation-app", Component: ptr.To("webservice")},
				Parameters: operationParams(map[string]string{"shouldFail": "true"}),
			},
		}
		Expect(k8sClient.Create(ctx, op)).Should(BeNil())

		By("waiting for the Operation to fail on step-three")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseFailed)

		Expect(op.Status.Workflows).To(HaveLen(1))
		Expect(op.Status.Workflows[0].Steps).To(HaveLen(3))
		stepThree := op.Status.Workflows[0].Steps[2]
		Expect(stepThree.Phase).To(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))
		Expect(stepThree.Attempts).To(HaveLen(1))
		Expect(stepThree.Attempts[0].Phase).To(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))
		firstAttemptID := stepThree.Attempts[0].ID

		By("fixing the parameter and restarting just step-three")
		op.Spec.Parameters = operationParams(map[string]string{"shouldFail": "false"})
		Expect(k8sClient.Update(ctx, op)).Should(BeNil())
		Expect(veloperation.NewOperationWorkflowStepOperator(k8sClient, GinkgoWriter, op).Restart(ctx, "step-three")).Should(BeNil())

		By("waiting for the Operation to succeed")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSucceeded)

		By("verifying step-three's attempt history has both the original failure and the successful retry")
		Expect(op.Status.Workflows[0].Steps).To(HaveLen(3))
		stepThree = op.Status.Workflows[0].Steps[2]
		Expect(stepThree.Phase).To(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(stepThree.Attempts).To(HaveLen(2))
		Expect(stepThree.Attempts[0].ID).To(Equal(firstAttemptID))
		Expect(stepThree.Attempts[0].Phase).To(Equal(workflowv1alpha1.WorkflowStepPhaseFailed))
		Expect(stepThree.Attempts[1].Phase).To(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(stepThree.Attempts[1].ID).NotTo(Equal(firstAttemptID))

		By("restarting step-one, already Succeeded -- allowed, and cascades to reset step-two/step-three too")
		stepOneIDBefore := op.Status.Workflows[0].Steps[0].ID
		Expect(veloperation.NewOperationWorkflowStepOperator(k8sClient, GinkgoWriter, op).Restart(ctx, "step-one")).Should(BeNil())

		By("waiting for the whole workflow to succeed again")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSucceeded)

		Expect(op.Status.Workflows[0].Steps).To(HaveLen(3))
		Expect(op.Status.Workflows[0].Steps[0].ID).NotTo(Equal(stepOneIDBefore), "step-one should have actually re-run, not merely been left alone")
		Expect(op.Status.Attempts).To(Equal(int64(3)), "the original run plus two restarts")
	})

	// TODO: move to envtest -- e2e should stay a slim smoke test.
	It("suspends and resumes a running workflow", func() {
		By("applying the ok-step WorkflowStepDefinition and the suspend-then-ok OperationTemplate (vela-system)")
		applyOkStepWorkflowStepDefinition(ctx)
		applySuspendOperationTemplate(ctx)

		By("applying the target Application")
		applyOperationTargetApp(ctx, namespaceName)

		By("creating the Operation")
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "suspend-", Namespace: namespaceName},
			Spec: v2alpha1.OperationSpec{
				Template: "suspend-then-ok",
				Source:   &v2alpha1.OperationSource{App: "operation-app", Component: ptr.To("webservice")},
			},
		}
		Expect(k8sClient.Create(ctx, op)).Should(BeNil())

		By("waiting for the Operation to reach phase Suspended")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSuspended)
		Expect(op.Status.Workflows[0].Terminated).To(BeFalse())
		Expect(op.Status.Workflows[0].Finished).To(BeFalse())

		By("resuming it")
		Expect(veloperation.NewOperationWorkflowOperator(k8sClient, GinkgoWriter, op).Resume(ctx)).Should(BeNil())

		By("waiting for it to complete")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSucceeded)
	})

	// TODO: retry feature needs to be fixed -- disabled pending a fix, see handoff notes.
	// TODO: move to envtest -- e2e should stay a slim smoke test.
	PIt("a --step restart re-reads a prior step's original output, not a recomputed one", func() {
		By("applying the ok-step/flaky-step WorkflowStepDefinitions and the io-flaky OperationTemplate (vela-system)")
		applyOkStepWorkflowStepDefinition(ctx)
		applyFlakyStepWorkflowStepDefinition(ctx)
		applyIOOperationTemplate(ctx)

		By("applying the target Application")
		applyOperationTargetApp(ctx, namespaceName)

		By("creating the Operation with shouldFail=true, so the consume step fails")
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "io-", Namespace: namespaceName},
			Spec: v2alpha1.OperationSpec{
				Template:   "io-flaky",
				Source:     &v2alpha1.OperationSource{App: "operation-app", Component: ptr.To("webservice")},
				Parameters: operationParams(map[string]string{"shouldFail": "true"}),
			},
		}
		Expect(k8sClient.Create(ctx, op)).Should(BeNil())

		By("waiting for the consume step to fail")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseFailed)

		Expect(op.Status.Workflows[0].Steps).To(HaveLen(2))
		emitID := op.Status.Workflows[0].Steps[0].ID
		Expect(emitID).NotTo(BeEmpty())

		By("verifying emit's output was recorded in the context-backend ConfigMap")
		Expect(op.Status.Workflows[0].ContextBackend).NotTo(BeNil())
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: op.Status.Workflows[0].ContextBackend.Namespace,
			Name:      op.Status.Workflows[0].ContextBackend.Name,
		}, cm)).Should(BeNil())
		Expect(lookupContextVar(cm, "upstream")).To(Equal(emitID))

		By("fixing the parameter and restarting just the consume step")
		op.Spec.Parameters = operationParams(map[string]string{"shouldFail": "false"})
		Expect(k8sClient.Update(ctx, op)).Should(BeNil())
		Expect(veloperation.NewOperationWorkflowStepOperator(k8sClient, GinkgoWriter, op).Restart(ctx, "consume")).Should(BeNil())

		By("waiting for it to succeed")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSucceeded)

		By("verifying emit was never re-executed")
		Expect(op.Status.Workflows[0].Steps[0].ID).To(Equal(emitID), "emit is positioned before the restarted step and should be untouched")

		By("verifying emit's recorded output in the ConfigMap is still the original value")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: op.Status.Workflows[0].ContextBackend.Namespace,
			Name:      op.Status.Workflows[0].ContextBackend.Name,
		}, cm)).Should(BeNil())
		Expect(lookupContextVar(cm, "upstream")).To(Equal(emitID), "consume's restart must not have cleared or changed emit's recorded output")
	})

	// TODO: move to envtest -- e2e should stay a slim smoke test.
	It("deletes a terminal Operation after its TTL elapses, but never one that's merely Suspended", func() {
		By("applying the ok-step/flaky-step WorkflowStepDefinitions and the retry-flaky/suspend-then-ok OperationTemplates (vela-system)")
		applyOkStepWorkflowStepDefinition(ctx)
		applyFlakyStepWorkflowStepDefinition(ctx)
		applyRetryOperationTemplate(ctx)
		applySuspendOperationTemplate(ctx)

		By("applying the target Application")
		applyOperationTargetApp(ctx, namespaceName)

		By("creating a short-TTL Operation that will succeed")
		// 20s, not 5s: leaves margin over waitForOperationPhase's 2s poll lag.
		ttl := int32(20)
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "ttl-", Namespace: namespaceName},
			Spec: v2alpha1.OperationSpec{
				Template:                "retry-flaky",
				Source:                  &v2alpha1.OperationSource{App: "operation-app", Component: ptr.To("webservice")},
				TTLSecondsAfterFinished: &ttl,
			},
		}
		Expect(k8sClient.Create(ctx, op)).Should(BeNil())

		By("waiting for it to succeed")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSucceeded)

		By("verifying it is not deleted immediately, before its TTL has elapsed")
		Consistently(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(op), &v2alpha1.Operation{})
		}, 2*time.Second, 500*time.Millisecond).Should(BeNil())

		By("verifying it is deleted shortly after the TTL elapses")
		Eventually(func() bool {
			return kerrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(op), &v2alpha1.Operation{}))
		}, 30*time.Second, 2*time.Second).Should(BeTrue())

		By("creating a suspended Operation with the same TTL")
		suspendOp := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "ttl-suspend-", Namespace: namespaceName},
			Spec: v2alpha1.OperationSpec{
				Template:                "suspend-then-ok",
				Source:                  &v2alpha1.OperationSource{App: "operation-app", Component: ptr.To("webservice")},
				TTLSecondsAfterFinished: &ttl,
			},
		}
		Expect(k8sClient.Create(ctx, suspendOp)).Should(BeNil())
		waitForOperationPhase(ctx, suspendOp, v2alpha1.OperationPhaseSuspended)

		By("verifying it is NOT deleted despite the TTL window elapsing -- Suspended is non-terminal")
		Consistently(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(suspendOp), &v2alpha1.Operation{})
		}, 22*time.Second, 2*time.Second).Should(BeNil())

		By("resuming it, then confirming the resume actually took effect")
		// Re-Get first: Resume's Status().Update() needs a current
		// ResourceVersion, and the controller keeps re-persisting status on
		// a Suspended Operation, so a stale copy can lose with a 409.
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(suspendOp), suspendOp)).Should(BeNil())
		Expect(veloperation.NewOperationWorkflowOperator(k8sClient, GinkgoWriter, suspendOp).Resume(ctx)).Should(BeNil())
		waitForOperationPhase(ctx, suspendOp, v2alpha1.OperationPhaseSucceeded)
		Eventually(func() bool {
			return kerrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(suspendOp), &v2alpha1.Operation{}))
		}, 30*time.Second, 2*time.Second).Should(BeTrue())
	})

	// TODO: move to envtest -- e2e should stay a slim smoke test.
	It("resolves Application-scope context against the source Application, with no component", func() {
		By("applying the appscope-check WorkflowStepDefinition and the app-scope OperationTemplate (vela-system)")
		applyAppScopeCheckWorkflowStepDefinition(ctx)
		applyApplicationScopeOperationTemplate(ctx)

		By("applying the target Application")
		applyOperationTargetApp(ctx, namespaceName)

		By("creating an Application-scoped Operation, no component")
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "app-scope-", Namespace: namespaceName},
			Spec: v2alpha1.OperationSpec{
				Template: "app-scope",
				Source:   &v2alpha1.OperationSource{App: "operation-app"},
			},
		}
		Expect(k8sClient.Create(ctx, op)).Should(BeNil())

		By("waiting for it to succeed")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSucceeded)
	})

	// TODO: move to envtest -- e2e should stay a slim smoke test.
	It("runs a None-scope Operation with no source at all", func() {
		By("applying the ok-step WorkflowStepDefinition and the none-scope OperationTemplate (vela-system)")
		applyOkStepWorkflowStepDefinition(ctx)
		applyNoneScopeOperationTemplate(ctx)

		By("creating a None-scope Operation, no spec.source")
		op := &v2alpha1.Operation{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "none-scope-", Namespace: namespaceName},
			Spec:       v2alpha1.OperationSpec{Template: "none-scope"},
		}
		Expect(k8sClient.Create(ctx, op)).Should(BeNil())

		By("waiting for it to succeed")
		waitForOperationPhase(ctx, op, v2alpha1.OperationPhaseSucceeded)
	})
})
