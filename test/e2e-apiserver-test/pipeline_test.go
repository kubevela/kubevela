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

package e2e_apiserver_test

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var testPipelineSteps []v1alpha1.WorkflowStep

func init() {
	rawProps := []byte(`{"url":"https://api.github.com/repos/kubevela/kubevela"}`)
	testPipelineSteps = []v1alpha1.WorkflowStep{
		{
			WorkflowStepBase: v1alpha1.WorkflowStepBase{
				Name: "request",
				Type: "request",
				Outputs: v1alpha1.StepOutputs{
					{
						ValueFrom: "import \"strconv\"\n\"Current star count: \" + strconv.FormatInt(response[\"stargazers_count\"], 10)\n",
						Name:      "stars",
					},
				},
				Properties: &runtime.RawExtension{
					Raw: rawProps,
				},
			},
		},
	}
}

var _ = Describe("Test the rest api about the pipeline", func() {
	var (
		projectName1    = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
		pipelineName    = "test-pipeline"
		contextName     = "test-context"
		contextKey      = "test-key"
		contextVal      = "test-val"
		pipelineRunName string
	)
	defer GinkgoRecover()
	It("create project and apply definitions", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateProjectRequest{
			Name:        projectName1,
			Description: "KubeVela Project",
		}
		res := post("/projects", req)
		var projectBase apisv1.ProjectBase
		Expect(decodeResponseBody(res, &projectBase)).Should(Succeed())
		Expect(cmp.Diff(projectBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(projectBase.Description, req.Description)).Should(BeEmpty())

		def1 := new(v1beta1.WorkflowStepDefinition)
		def2 := new(v1beta1.WorkflowStepDefinition)
		Expect(common.ReadYamlToObject("./testdata/request.yaml", def1)).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), def1)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(common.ReadYamlToObject("./testdata/log.yaml", def2)).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), def2)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	It("create pipeline", func() {
		var req = apisv1.CreatePipelineRequest{
			Name: "test-pipeline",
			Spec: v1alpha1.WorkflowSpec{
				Steps: testPipelineSteps,
			},
		}
		res := post("/projects/"+projectName1+"/pipelines", req)
		var pipeline apisv1.PipelineBase
		Expect(decodeResponseBody(res, &pipeline)).Should(Succeed())
		Expect(cmp.Diff(pipeline.Name, req.Name)).Should(BeEmpty())
		Expect(len(pipeline.Spec.Steps)).Should(Equal(len(req.Spec.Steps)))
	})

	It("create context", func() {
		var req = apisv1.CreateContextValuesRequest{
			Name: contextName,
			Values: []model.Value{
				{
					Key:   contextKey,
					Value: contextVal,
				},
			},
		}
		res := post("/projects/"+projectName1+"/pipelines/"+pipelineName+"/contexts", req)
		var context apisv1.Context
		Expect(decodeResponseBody(res, &context)).Should(Succeed())
		Expect(cmp.Diff(context.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(context.Values, req.Values)).Should(BeEmpty())
	})

	It("get contexts", func() {
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/contexts")
		var contexs apisv1.ListContextValueResponse
		Expect(decodeResponseBody(res, &contexs)).Should(Succeed())
		Expect(len(contexs.Contexts)).Should(Equal(1))
		ctx, ok := contexs.Contexts[contextName]
		Expect(ok).Should(BeTrue())
		Expect(len(ctx)).Should(Equal(1))
	})

	It("update context", func() {
		var req = apisv1.UpdateContextValuesRequest{
			Values: []model.Value{
				{
					Key:   contextKey,
					Value: "new-val",
				},
			},
		}
		res := put("/projects/"+projectName1+"/pipelines/"+pipelineName+"/contexts/"+contextName, req)
		var context apisv1.Context
		Expect(res.StatusCode).Should(Equal(http.StatusOK))
		Expect(decodeResponseBody(res, &context)).Should(Succeed())

		By("check the context value")
		Expect(cmp.Diff(context.Values[0].Value, "new-val")).Should(BeEmpty())
	})

	It("update pipeline", func() {
		newSteps := make([]v1alpha1.WorkflowStep, 0)
		newSteps = append(newSteps, *testPipelineSteps[0].DeepCopy())
		newSteps = append(newSteps, v1alpha1.WorkflowStep{
			WorkflowStepBase: v1alpha1.WorkflowStepBase{
				Name:      "log",
				Type:      "log",
				DependsOn: nil,
				Inputs: v1alpha1.StepInputs{
					{
						ParameterKey: "data",
						From:         "stars",
					},
				},
			},
		})
		var req = apisv1.UpdatePipelineRequest{
			Spec: v1alpha1.WorkflowSpec{
				Steps: newSteps,
			},
		}
		res := put("/projects/"+projectName1+"/pipelines/"+pipelineName, req)
		var pipeline apisv1.PipelineBase
		Expect(decodeResponseBody(res, &pipeline)).Should(Succeed())
		Expect(len(pipeline.Spec.Steps)).Should(Equal(len(req.Spec.Steps)))
	})

	It("run pipeline", func() {
		var req = apisv1.RunPipelineRequest{
			Mode: v1alpha1.WorkflowExecuteMode{
				Steps:    "StepByStep",
				SubSteps: "DAG",
			},
			ContextName: contextName,
		}
		res := post("/projects/"+projectName1+"/pipelines/"+pipelineName+"/run", req)
		var run apisv1.PipelineRun
		Expect(decodeResponseBody(res, &run)).Should(Succeed())
		Expect(run.PipelineRunName).ShouldNot(BeEmpty())
		pipelineRunName = run.PipelineRunName
	})

	It("list pipeline", func() {
		res := get("/pipelines")
		var pipelines apisv1.ListPipelineResponse
		Expect(decodeResponseBody(res, &pipelines)).Should(Succeed())
		Expect(pipelines.Total).Should(BeNumerically("==", 1))
		Expect(pipelines.Pipelines[0].Name).Should(Equal("test-pipeline"))
	})

	It("get pipeline", func() {
		Eventually(func(g Gomega) {
			res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName)
			var pipeline apisv1.GetPipelineResponse
			Expect(decodeResponseBody(res, &pipeline)).Should(Succeed())
			Expect(pipeline.Name).Should(Equal("test-pipeline"))
			Expect(pipeline.PipelineInfo.LastRun).ShouldNot(BeNil())
			Expect(pipeline.PipelineInfo.RunStat.Total).Should(Equal(apisv1.RunStatInfo{Total: 1, Success: 1}))
			Expect(len(pipeline.PipelineInfo.RunStat.Week)).Should(Equal(7))
		}, 10*time.Second, 1*time.Second).Should(Succeed())
	})

	It("list pipeline runs", func() {
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs")
		var runs apisv1.ListPipelineRunResponse
		Expect(decodeResponseBody(res, &runs)).Should(Succeed())
		Expect(runs.Total).Should(BeNumerically("==", 1))
	})

	It("get pipeline run", func() {
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs/" + pipelineRunName)
		var run apisv1.PipelineRunBase
		Expect(decodeResponseBody(res, &run)).Should(Succeed())
		Expect(run.PipelineRunName).Should(Equal(pipelineRunName))
	})

	It("get pipeline run status", func() {
		Eventually(func(g Gomega) {
			res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs/" + pipelineRunName + "/status")
			var status v1alpha1.WorkflowRunStatus
			g.Expect(decodeResponseBody(res, &status)).Should(Succeed())
			g.Expect(status.Finished).Should(Equal(true))
			g.Expect(status.Phase).Should(Equal(v1alpha1.WorkflowStateSucceeded))
			g.Expect(status.Message).Should(BeEmpty())
		}, 100*time.Second, 1*time.Second).Should(Succeed())
	})

	It("get pipeline run output", func() {
		outputStep := "request"
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs/" + pipelineRunName + "/output?step=" + outputStep)
		var output apisv1.GetPipelineRunOutputResponse
		Expect(decodeResponseBody(res, &output)).Should(Succeed())
		Expect(output.StepOutputs).Should(HaveLen(1))
		Expect(output.StepOutputs[0].Name).Should(Equal(outputStep))
		Expect(output.StepOutputs[0].Values).Should(HaveLen(1))
		Expect(output.StepOutputs[0].Values[0].Value).ShouldNot(BeEmpty())
	})

	It("get pipeline run input", func() {
		inputStep := "log"
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs/" + pipelineRunName + "/input?step=" + inputStep)
		var input apisv1.GetPipelineRunInputResponse
		Expect(decodeResponseBody(res, &input)).Should(Succeed())
		Expect(input.StepInputs).Should(HaveLen(1))
		Expect(input.StepInputs[0].Name).Should(Equal(inputStep))
		Expect(input.StepInputs[0].Values).Should(HaveLen(1))
		Expect(input.StepInputs[0].Values[0].Value).ShouldNot(BeEmpty())
	})

	It("get pipeline run logs", func() {
		logStep := "log"
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs/" + pipelineRunName + "/log?step=" + logStep)
		var logs apisv1.GetPipelineRunLogResponse
		Expect(decodeResponseBody(res, &logs)).Should(Succeed())
		Expect(logs.Name).Should(Equal(logStep))
		Expect(logs.Log).ShouldNot(BeEmpty())
	})

	It("delete pipeline run", func() {
		res := delete("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs/" + pipelineRunName)
		Expect(res.StatusCode).Should(Equal(http.StatusOK))
	})

	It("delete context", func() {
		res := delete("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/contexts/" + contextName)
		Expect(res.StatusCode).Should(Equal(http.StatusOK))
	})

	It("delete pipeline", func() {
		res := delete("/projects/" + projectName1 + "/pipelines/" + pipelineName)
		Expect(res.StatusCode).Should(Equal(http.StatusOK))
	})
})
