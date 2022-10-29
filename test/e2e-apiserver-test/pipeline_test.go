package e2e_apiserver_test

import (
	"encoding/json"
	"github.com/google/go-cmp/cmp"
	"github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"strconv"
	"time"

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
		projectName1 = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
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
		res := get("/projects/" + projectName1 + "/pipelines/test-pipeline")
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
		res := put("/projects/"+projectName1+"/pipelines/test-pipeline", req)
		var pipeline apisv1.PipelineBase
		Expect(decodeResponseBody(res, &pipeline)).Should(Succeed())
		Expect(cmp.Diff(pipeline.Spec, req.Spec)).Should(BeEmpty())
	})

	It("run pipeline", func() {

	})

	It("list pipeline runs", func() {

	})

	It("delete pipeline", func() {

	})

})
