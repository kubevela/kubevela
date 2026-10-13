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

package mcp

import (
	"time"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func raw(s string) *runtime.RawExtension { return &runtime.RawExtension{Raw: []byte(s)} }

// webApp is a two-component Application in team-a with a suspended
// approval step; its status is one generation behind the spec.
func webApp() *v1beta1.Application {
	mode := wfTypesv1alpha1.WorkflowExecuteMode{Steps: wfTypesv1alpha1.WorkflowMode("DAG")}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "web",
			Namespace:         "team-a",
			UID:               "app-web-uid",
			Generation:        3,
			ResourceVersion:   "1234",
			CreationTimestamp: metav1.NewTime(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)),
			Labels:            map[string]string{"team": "a"},
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []oamcommon.ApplicationComponent{
				{
					Name:       "web",
					Type:       "webservice",
					Properties: raw(`{"image":"nginx:1.27","env":[{"name":"DB_PASSWORD","value":"hunter2"}],"apiToken":"abc123"}`),
					Traits:     []oamcommon.ApplicationTrait{{Type: "scaler", Properties: raw(`{"replicas":2}`)}},
				},
				{Name: "db", Type: "helm", DependsOn: []string{"web"}, Properties: raw(`{"chart":"postgres","password":"pg-secret"}`)},
			},
			Policies: []v1beta1.AppPolicy{{Name: "topo", Type: "topology", Properties: raw(`{"clusters":["local"]}`)}},
			Workflow: &v1beta1.Workflow{
				Mode: &mode,
				Steps: []wfTypesv1alpha1.WorkflowStep{
					{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "deploy-web", Type: "deploy", Properties: raw(`{"policies":["topo"]}`)}},
					{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "approve", Type: "suspend", DependsOn: []string{"deploy-web"}}},
					{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "deploy-db", Type: "deploy", If: "always", Timeout: "5m"}},
				},
			},
		},
		Status: oamcommon.AppStatus{
			ConditionedStatus:  condition.ConditionedStatus{Conditions: []condition.Condition{{Type: "Parsed", Status: corev1.ConditionTrue, Reason: "Available"}}},
			ObservedGeneration: 2,
			Phase:              oamcommon.ApplicationWorkflowSuspending,
			LatestRevision:     &oamcommon.Revision{Name: "web-v2", Revision: 2},
			Services: []oamcommon.ApplicationComponentStatus{{
				Name: "web", Cluster: "local", Namespace: "team-a", Healthy: true, Message: "Ready:2/2",
				WorkloadDefinition: oamcommon.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"},
				Traits:             []oamcommon.ApplicationTraitStatus{{Type: "scaler", Healthy: true}},
			}},
			AppliedResources: []oamcommon.ClusterObjectReference{{
				Creator:         "workflow",
				ObjectReference: corev1.ObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "web"},
			}},
			Workflow: &oamcommon.WorkflowStatus{
				AppRevision: "web-v2",
				Mode:        "DAG-DAG",
				Phase:       workflowv1alpha1.WorkflowStateSuspending,
				Suspend:     true,
				StartTime:   metav1.NewTime(time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)),
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{StepStatus: workflowv1alpha1.StepStatus{ID: "s1", Name: "deploy-web", Type: "deploy", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
					{StepStatus: workflowv1alpha1.StepStatus{ID: "s2", Name: "approve", Type: "suspend", Phase: workflowv1alpha1.WorkflowStepPhaseSuspending, Message: "Suspended by field suspend"}},
					{StepStatus: workflowv1alpha1.StepStatus{ID: "s3", Name: "deploy-db", Type: "deploy", Phase: workflowv1alpha1.WorkflowStepPhasePending}},
				},
			},
		},
	}
}

// failedApp is a running Application in team-b whose only step failed.
func failedApp() *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "batch", Namespace: "team-b", Generation: 1},
		Spec:       v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{{Name: "job", Type: "task"}}},
		Status: oamcommon.AppStatus{
			ObservedGeneration: 1,
			Phase:              oamcommon.ApplicationWorkflowFailed,
			Workflow: &oamcommon.WorkflowStatus{
				Mode: "StepByStep-DAG", Phase: workflowv1alpha1.WorkflowStateFailed, Terminated: true,
				Steps: []workflowv1alpha1.WorkflowStepStatus{{StepStatus: workflowv1alpha1.StepStatus{Name: "run", Type: "deploy", Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Reason: "Timeout"}}},
			},
		},
	}
}

func appFixtures() []client.Object {
	return append(platformFixtures(), webApp(), failedApp())
}

var _ = Describe("application tools", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	Context("list_applications", func() {
		It("lists every application with health, workflow state and staleness", func() {
			h = startHarness(Options{}, appFixtures()...)
			var out ListApplicationsOutput
			res := h.call("list_applications", nil, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.Applications).To(HaveLen(2))
			web := out.Applications[0]
			Expect(web.Name).To(Equal("web"))
			Expect(web.Phase).To(Equal("workflowSuspending"))
			Expect(web.Healthy).To(BeTrue())
			Expect(web.StatusStale).To(BeTrue())
			Expect(web.Revision).To(Equal("web-v2"))
			Expect(web.Components).To(ConsistOf(ComponentRef{Name: "web", Type: "webservice"}, ComponentRef{Name: "db", Type: "helm"}))
			Expect(web.Workflow.Suspended).To(BeTrue())
			batch := out.Applications[1]
			Expect(batch.Healthy).To(BeFalse())
			Expect(batch.StatusStale).To(BeFalse())
			Expect(batch.Workflow.Terminated).To(BeTrue())
			Expect(out.Meta.Source).To(Equal(SourceLive))
		})

		It("filters by namespace and enforces the namespace restriction", func() {
			h = startHarness(Options{Namespaces: []string{"team-b"}}, appFixtures()...)
			var out ListApplicationsOutput
			h.call("list_applications", nil, &out)
			Expect(out.Applications).To(HaveLen(1))
			Expect(out.Applications[0].Namespace).To(Equal("team-b"))

			res := h.call("list_applications", map[string]any{"namespace": "team-a"}, nil)
			Expect(errorText(res)).To(ContainSubstring("outside this server's allowed namespaces"))
		})
	})

	Context("get_application", func() {
		It("returns the redacted spec and the recorded status", func() {
			h = startHarness(Options{}, appFixtures()...)
			var out GetApplicationOutput
			res := h.call("get_application", map[string]any{"name": "web", "namespace": "team-a"}, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.Labels).To(HaveKeyWithValue("team", "a"))
			Expect(out.Spec.Components).To(HaveLen(2))
			web := out.Spec.Components[0]
			Expect(web.Properties).To(HaveKeyWithValue("image", "nginx:1.27"))
			Expect(web.Properties).To(HaveKeyWithValue("apiToken", redactedValue))
			env := web.Properties["env"].([]any)[0].(map[string]any)
			Expect(env).To(HaveKeyWithValue("name", "DB_PASSWORD"))
			Expect(env).To(HaveKeyWithValue("value", "hunter2"), "only keys are matched, env values are a later redaction pass")
			Expect(web.Traits[0].Properties).To(HaveKeyWithValue("replicas", BeEquivalentTo(2)))
			Expect(out.Spec.Components[1].Properties).To(HaveKeyWithValue("password", redactedValue))
			Expect(out.Spec.Components[1].DependsOn).To(ConsistOf("web"))
			Expect(out.Spec.Policies[0].Type).To(Equal("topology"))
			Expect(out.Spec.Workflow.Mode).To(Equal("DAG"))
			Expect(out.Spec.Workflow.Steps).To(HaveLen(3))
			Expect(out.Spec.Workflow.Steps[2].If).To(Equal("always"))

			Expect(out.Status.Phase).To(Equal("workflowSuspending"))
			Expect(out.Status.Generation).To(Equal(int64(3)))
			Expect(out.Status.ObservedGeneration).To(Equal(int64(2)))
			Expect(out.Status.Services).To(HaveLen(1))
			Expect(out.Status.Services[0].Workload).To(Equal("apps/v1/Deployment"))
			Expect(out.Status.Services[0].Traits[0].Healthy).To(BeTrue())
			Expect(out.Status.Conditions[0].Type).To(Equal("Parsed"))
			Expect(out.Status.AppliedResources[0].Cluster).To(Equal("local"))
			Expect(out.Status.Workflow.SuspendedSteps).To(ConsistOf("approve"))
			Expect(out.Evidence).To(ContainElement(ContainSubstring("Application team-a/web generation 3 resourceVersion 1234")))
		})

		It("requires a namespace and reports a missing application", func() {
			h = startHarness(Options{}, appFixtures()...)
			// The SDK validates arguments against the input schema before the
			// handler runs, so the message is the schema's, not validate()'s.
			res := h.call("get_application", map[string]any{"name": "web"}, nil)
			Expect(errorText(res)).To(ContainSubstring(`"namespace"`))
			res = h.call("get_application", map[string]any{"name": "ghost", "namespace": "team-a"}, nil)
			Expect(errorText(res)).To(ContainSubstring("not found"))
		})
	})

	Context("get_workflow", func() {
		It("names the suspended step and the resume command", func() {
			h = startHarness(Options{}, appFixtures()...)
			var out GetWorkflowOutput
			h.call("get_workflow", map[string]any{"name": "web", "namespace": "team-a"}, &out)
			Expect(out.Spec.Steps).To(HaveLen(3))
			Expect(out.Status.Suspended).To(BeTrue())
			Expect(out.Status.Steps).To(HaveLen(3))
			Expect(out.Status.Steps[1].Phase).To(Equal("suspending"))
			Expect(out.Status.SuspendedSteps).To(ConsistOf("approve"))
			Expect(out.Status.FailedSteps).To(BeEmpty())
			Expect(out.Status.NextAction).To(Equal("vela workflow resume web -n team-a"))
			Expect(out.Status.StartTime).NotTo(BeNil())
			Expect(out.Status.EndTime).To(BeNil())
		})

		It("names the failed step and the restart command", func() {
			h = startHarness(Options{}, appFixtures()...)
			var out GetWorkflowOutput
			h.call("get_workflow", map[string]any{"name": "batch", "namespace": "team-b"}, &out)
			Expect(out.Spec).To(BeNil())
			Expect(out.Status.FailedSteps).To(ConsistOf("run"))
			Expect(out.Status.Steps[0].Reason).To(Equal("Timeout"))
			Expect(out.Status.NextAction).To(Equal("vela workflow restart batch -n team-b"))
		})
	})
})
