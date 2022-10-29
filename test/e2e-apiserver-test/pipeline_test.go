package e2e_apiserver_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

var testPipelineSteps []v1alpha1.WorkflowStep

func init() {
	props := map[string]string{"url": "https://api.github.com/repos/kubevela/kubevela"}
	rawProps, err := json.Marshal(props)
	if err != nil {
		panic(err)
	}
	testPipelineSteps = []v1alpha1.WorkflowStep{
		{
			WorkflowStepBase: v1alpha1.WorkflowStepBase{
				Name: "request",
				// todo add request definition
				Type: "request",
				Outputs: v1alpha1.StepOutputs{
					{
						ValueFrom: "import \\\"strconv\\\"\\n\\\"Current star count: \\\" + strconv.FormatInt(response[\\\"stargazers_count\\\"], 10)\\n",
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

var _ = FDescribe("Test the rest api about the pipeline", func() {
	var (
		projectName1    = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
		pipelineName    = "test-pipeline"
		pipelineRunName string
	)
	defer GinkgoRecover()
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
		Expect(cmp.Diff(pipeline.Spec, req.Spec)).Should(BeEmpty())
	})

	It("list pipeline", func() {
		res := get("/pipelines")
		var pipelines apisv1.ListPipelineResponse
		Expect(decodeResponseBody(res, &pipelines)).Should(Succeed())
		Expect(pipelines.Total).Should(BeNumerically("=", 1))
		Expect(pipelines.Pipelines[0].Name).Should(Equal("test-pipeline"))
	})

	It("get pipeline", func() {
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName)
		var pipeline apisv1.GetPipelineResponse
		{
		}
		Expect(decodeResponseBody(res, &pipeline)).Should(Succeed())
		Expect(pipeline.Name).Should(Equal("test-pipeline"))
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
		Expect(cmp.Diff(pipeline.Spec, req.Spec)).Should(BeEmpty())
	})

	It("run pipeline", func() {
		var req = apisv1.RunPipelineRequest{
			Mode: v1alpha1.WorkflowExecuteMode{
				Steps:    "StepByStep",
				SubSteps: "DAG",
			},
			ContextName: "",
		}
		res := post("/projects/"+projectName1+"/pipelines/"+pipelineName+"/run", req)
		var run apisv1.PipelineRun
		Expect(decodeResponseBody(res, &run)).Should(Succeed())
		Expect(run.PipelineRunName).ShouldNot(BeEmpty())
		Eventually(func() bool {
			res := post("/projects/"+projectName1+"/pipelines/"+pipelineName+"/run", req)
			Expect(decodeResponseBody(res, &run)).Should(Succeed())
			return run.Status.Finished != true
		}, 5*time.Second, 1*time.Second).Should(BeTrue())
	})

	It("list pipeline runs", func() {
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs")
		var runs apisv1.ListPipelineRunResponse
		Expect(decodeResponseBody(res, &runs)).Should(Succeed())
		Expect(runs.Total).Should(BeNumerically("=", 1))
		pipelineRunName = runs.Runs[0].PipelineRunName
	})

	It("get pipeline run", func() {
		res := get("/projects/" + projectName1 + "/pipelines/" + pipelineName + "/runs/" + pipelineRunName)
		var run apisv1.PipelineRunBase
		Expect(decodeResponseBody(res, &run)).Should(Succeed())
		Expect(run.PipelineRunName).Should(Equal(pipelineRunName))
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

	It("delete pipeline", func() {
		res := delete("/projects/" + projectName1 + "/pipelines/" + pipelineName)
		Expect(res.StatusCode).Should(Equal(http.StatusOK))
	})
})
