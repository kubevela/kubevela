/*
Copyright 2022 The KubeVela Authors.

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

package model

import (
	"context"
	"testing"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("test Application", func() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "")

	It("application num", func() {
		num, err := applicationNum(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(num).To(Equal(1))
	})
	It("running application num", func() {
		num, err := runningApplicationNum(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(num).To(Equal(1))
	})
	It("application running ratio", func() {
		num := ApplicationRunningNum(cfg)
		Expect(num).To(Equal("1/1"))
	})
	It("list applications", func() {
		applicationsList, err := ListApplications(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(applicationsList)).To(Equal(1))
	})
	It("load application info", func() {
		application, err := LoadApplication(k8sClient, "first-vela-app", "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(application.Name).To(Equal("first-vela-app"))
		Expect(application.Namespace).To(Equal("default"))
		Expect(len(application.Spec.Components)).To(Equal(1))
	})
	It("application resource topology", func() {
		topology, err := ApplicationResourceTopology(k8sClient, "first-vela-app", "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(len(topology)).To(Equal(4))
	})
})

func TestApplicationList_ToTableBody(t *testing.T) {
	testCases := []struct {
		name     string
		list     ApplicationList
		expected [][]string
	}{
		{
			name:     "empty list",
			list:     ApplicationList{},
			expected: make([][]string, 0),
		},
		{
			name: "single item list",
			list: ApplicationList{
				{name: "app1", namespace: "ns1", phase: "running", workflowMode: "DAG", workflow: "1/1", service: "1/1", createTime: "now"},
			},
			expected: [][]string{
				{"app1", "ns1", "running", "DAG", "1/1", "1/1", "now"},
			},
		},
		{
			name: "multiple item list",
			list: ApplicationList{
				{name: "app1", namespace: "ns1", phase: "running", workflowMode: "DAG", workflow: "1/1", service: "1/1", createTime: "now"},
				{name: "app2", namespace: "ns2", phase: "failed", workflowMode: "StepByStep", workflow: "0/1", service: "0/1", createTime: "then"},
			},
			expected: [][]string{
				{"app1", "ns1", "running", "DAG", "1/1", "1/1", "now"},
				{"app2", "ns2", "failed", "StepByStep", "0/1", "0/1", "then"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.list.ToTableBody()
			if len(tc.expected) == 0 {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestServiceNum(t *testing.T) {
	testCases := []struct {
		name     string
		app      v1beta1.Application
		expected string
	}{
		{
			name:     "no services",
			app:      v1beta1.Application{Status: common.AppStatus{Services: []common.ApplicationComponentStatus{}}},
			expected: "0/0",
		},
		{
			name: "one healthy, one unhealthy",
			app: v1beta1.Application{Status: common.AppStatus{Services: []common.ApplicationComponentStatus{
				{Healthy: true},
				{Healthy: false},
			}}},
			expected: "1/2",
		},
		{
			name: "all healthy",
			app: v1beta1.Application{Status: common.AppStatus{Services: []common.ApplicationComponentStatus{
				{Healthy: true},
				{Healthy: true},
			}}},
			expected: "2/2",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, serviceNum(tc.app))
		})
	}
}

func TestWorkflowMode(t *testing.T) {
	testCases := []struct {
		name     string
		app      v1beta1.Application
		expected string
	}{
		{
			name:     "workflow is nil",
			app:      v1beta1.Application{Status: common.AppStatus{Workflow: nil}},
			expected: Unknown,
		},
		{
			name:     "workflow mode is DAG",
			app:      v1beta1.Application{Status: common.AppStatus{Workflow: &common.WorkflowStatus{Mode: "DAG"}}},
			expected: "DAG",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, workflowMode(tc.app))
		})
	}
}

func TestWorkflowStepNum(t *testing.T) {
	testCases := []struct {
		name     string
		app      v1beta1.Application
		expected string
	}{
		{
			name:     "workflow is nil",
			app:      v1beta1.Application{Status: common.AppStatus{Workflow: nil}},
			expected: "N/A",
		},
		{
			name: "empty workflow steps",
			app: v1beta1.Application{Status: common.AppStatus{Workflow: &common.WorkflowStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{},
			}}},
			expected: "0/0",
		},
		{
			name: "all steps succeeded",
			app: v1beta1.Application{Status: common.AppStatus{Workflow: &common.WorkflowStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{StepStatus: workflowv1alpha1.StepStatus{Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
					{StepStatus: workflowv1alpha1.StepStatus{Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
				},
			}}},
			expected: "2/2",
		},
		{
			name: "some steps succeeded, some failed/running",
			app: v1beta1.Application{Status: common.AppStatus{Workflow: &common.WorkflowStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{StepStatus: workflowv1alpha1.StepStatus{Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}},
					{StepStatus: workflowv1alpha1.StepStatus{Phase: workflowv1alpha1.WorkflowStepPhaseFailed}},
					{StepStatus: workflowv1alpha1.StepStatus{Phase: workflowv1alpha1.WorkflowStepPhaseRunning}},
				},
			}}},
			expected: "1/3",
		},
		{
			name: "all steps failed/running",
			app: v1beta1.Application{Status: common.AppStatus{Workflow: &common.WorkflowStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{StepStatus: workflowv1alpha1.StepStatus{Phase: workflowv1alpha1.WorkflowStepPhaseFailed}},
					{StepStatus: workflowv1alpha1.StepStatus{Phase: workflowv1alpha1.WorkflowStepPhaseRunning}},
				},
			}}},
			expected: "0/2",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, workflowStepNum(tc.app))
		})
	}
}
