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

package workflow

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

var _ = Describe("Test Workflow", func() {

	BeforeEach(func() {
		cm := &corev1.ConfigMap{}
		revJson, err := yaml.YAMLToJSON([]byte(revYaml))
		Expect(err).ToNot(HaveOccurred())
		err = json.Unmarshal(revJson, cm)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Create(context.Background(), cm)
		if err != nil && !kerrors.IsAlreadyExists(err) {
			Expect(err).ToNot(HaveOccurred())
		}

	})
	It("Workflow test for failed", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "failed",
			},
			{
				Name: "s3",
				Type: "success",
			},
		})
		wf := NewWorkflow(app, k8sClient)
		done, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())

		Expect(done).Should(BeFalse())
		workflowStatus := app.Status.Workflow
		Expect(workflowStatus.ContextBackend.Name).Should(BeEquivalentTo("workflow-" + revision))
		workflowStatus.ContextBackend = nil
		Expect(cmp.Diff(*workflowStatus, common.WorkflowStatus{
			AppRevision: revision,
			StepIndex:   1,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "failed",
				Phase: common.WorkflowStepPhaseFailed,
			}},
		})).Should(BeEquivalentTo(""))

		app, runners = makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "success",
			},
			{
				Name: "s3",
				Type: "success",
			},
		})

		app.Status.Workflow = workflowStatus
		wf = NewWorkflow(app, k8sClient)
		done, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: revision,
			StepIndex:   3,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s3",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))

	})

	It("test for suspend", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "suspend",
			},
			{
				Name: "s3",
				Type: "success",
			},
		})
		wf := NewWorkflow(app, k8sClient)
		done, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: revision,
			StepIndex:   2,
			Suspend:     true,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "suspend",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))

		app.Status.Workflow.Suspend = false
		done, err = wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: revision,
			StepIndex:   3,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "suspend",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s3",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))
	})

	It("test for terminate", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "terminate",
			},
		})
		wf := NewWorkflow(app, k8sClient)
		done, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).ToNot(HaveOccurred())
		Expect(done).Should(BeTrue())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: revision,
			StepIndex:   2,
			Terminated:  true,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, {
				Name:  "s2",
				Type:  "terminate",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))
	})

	It("test for error", func() {
		app, runners := makeTestCase([]oamcore.WorkflowStep{
			{
				Name: "s1",
				Type: "success",
			},
			{
				Name: "s2",
				Type: "error",
			},
		})
		wf := NewWorkflow(app, k8sClient)
		done, err := wf.ExecuteSteps(context.Background(), revision, runners)
		Expect(err).To(HaveOccurred())
		Expect(done).Should(BeFalse())
		app.Status.Workflow.ContextBackend = nil
		Expect(cmp.Diff(*app.Status.Workflow, common.WorkflowStatus{
			AppRevision: revision,
			StepIndex:   1,
			Steps: []common.WorkflowStepStatus{{
				Name:  "s1",
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}},
		})).Should(BeEquivalentTo(""))
	})
})

func makeTestCase(steps []oamcore.WorkflowStep) (*oamcore.Application, []wfTypes.TaskRunner) {
	app := &oamcore.Application{
		Spec: oamcore.ApplicationSpec{
			Workflow: &oamcore.Workflow{
				Steps: steps,
			},
		},
		Status: common.AppStatus{},
	}
	app.Namespace = "default"
	app.Name = "app"
	runners := []wfTypes.TaskRunner{}
	for _, step := range steps {
		runners = append(runners, makeRunner(step.Name, step.Type))
	}
	return app, runners
}

func makeRunner(name, tpy string) wfTypes.TaskRunner {
	switch tpy {
	case "suspend":
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
					Name:  name,
					Type:  "suspend",
					Phase: common.WorkflowStepPhaseSucceeded,
				}, &wfTypes.Operation{
					Suspend: true,
				}, nil
		}
	case "terminate":
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
					Name:  name,
					Type:  "terminate",
					Phase: common.WorkflowStepPhaseSucceeded,
				}, &wfTypes.Operation{
					Terminated: true,
				}, nil
		}
	case "success":
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  "success",
				Phase: common.WorkflowStepPhaseSucceeded,
			}, &wfTypes.Operation{}, nil
		}
	case "failed":
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  "failed",
				Phase: common.WorkflowStepPhaseFailed,
			}, &wfTypes.Operation{}, nil
		}
	case "error":
		return func(ctx wfContext.Context) (common.WorkflowStepStatus, *wfTypes.Operation, error) {
			return common.WorkflowStepStatus{
				Name:  name,
				Type:  "error",
				Phase: common.WorkflowStepPhaseRunning,
			}, &wfTypes.Operation{}, errors.New("error for test")
		}
	}
	return nil
}

var (
	revYaml = `apiVersion: v1
data:
  components: '{"server":"{\"Scopes\":null,\"StandardWorkload\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Pod\\\",\\\"metadata\\\":{\\\"labels\\\":{\\\"app\\\":\\\"nginx\\\"}},\\\"spec\\\":{\\\"containers\\\":[{\\\"env\\\":[{\\\"name\\\":\\\"APP\\\",\\\"value\\\":\\\"nginx\\\"}],\\\"image\\\":\\\"nginx:1.14.2\\\",\\\"imagePullPolicy\\\":\\\"IfNotPresent\\\",\\\"name\\\":\\\"main\\\",\\\"ports\\\":[{\\\"containerPort\\\":8080,\\\"protocol\\\":\\\"TCP\\\"}]}]}}\",\"Traits\":[\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Service\\\",\\\"metadata\\\":{\\\"name\\\":\\\"my-service\\\"},\\\"spec\\\":{\\\"ports\\\":[{\\\"port\\\":80,\\\"protocol\\\":\\\"TCP\\\",\\\"targetPort\\\":8080}],\\\"selector\\\":{\\\"app\\\":\\\"nginx\\\"}}}\"]}"}'
kind: ConfigMap
metadata:
  name: app-v1
  namespace: default
`
	revision = "app-v1"
)