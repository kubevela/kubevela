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

package e2e_apiserver_test

import (
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

var _ = Describe("Test application workflow rest api", func() {
	var appName = "test-workflow"
	var envName = "workflow"
	var recordName = ""
	It("Prepare the environment", func() {
		ct := apisv1.CreateTargetRequest{
			Name:    "workflow",
			Project: appProject,
			Cluster: &apisv1.ClusterTarget{
				ClusterName: "local",
				Namespace:   "workflow",
			},
		}
		res := post("/targets", ct)
		var targetBase apisv1.TargetBase
		Expect(decodeResponseBody(res, &targetBase)).Should(Succeed())

		ce := apisv1.CreateEnvRequest{
			Name:      envName,
			Project:   appProject,
			Namespace: "workflow",
			Targets:   []string{targetBase.Name},
		}

		env := post("/envs", ce)
		var envRes apisv1.Env
		Expect(decodeResponseBody(env, &envRes)).Should(Succeed())
	})

	It("Create the application", func() {
		var req = apisv1.CreateApplicationRequest{
			Name:        appName,
			Project:     appProject,
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			EnvBinding:  []*apisv1.EnvBinding{{Name: envName}},
			Component: &apisv1.CreateComponentRequest{
				Name:          "c1",
				ComponentType: "webservice",
				Properties:    "{\"image\":\"nginx\"}",
			},
		}
		res := post("/applications", req)
		var appBase apisv1.ApplicationBase
		Expect(decodeResponseBody(res, &appBase)).Should(Succeed())
		Expect(cmp.Diff(appBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Labels["test"], req.Labels["test"])).Should(BeEmpty())
	})

	It("Update the workflow", func() {
		res := get(fmt.Sprintf("/applications/%s/workflows/%s", appName, repository.ConvertWorkflowName(envName)))
		var workflow apisv1.DetailWorkflowResponse
		Expect(decodeResponseBody(res, &workflow)).Should(Succeed())

		var defaultW = true
		uwr := &apisv1.UpdateWorkflowRequest{
			Alias:       "First Workflow",
			Description: "This is a workflow",
			Steps: append(workflow.Steps, apisv1.WorkflowStep{WorkflowStepBase: apisv1.WorkflowStepBase{
				Name: "group",
				Type: "step-group",
			}, SubSteps: []apisv1.WorkflowStepBase{{
				Name: "create-config",
				Type: "create-config",
				Properties: map[string]interface{}{
					"name": "demo",
					"config": map[string]interface{}{
						"workflowName": workflow.Name,
						"configName":   "demo",
					},
				},
			}, {
				Name: "read-config",
				Type: "read-config",
				Properties: map[string]interface{}{
					"name": "demo",
				},
				DependsOn: []string{"create-config"},
				Outputs: v1alpha1.StepOutputs{{
					ValueFrom: "output.config.configName",
					Name:      "configName",
				}},
			}, {
				Name: "delete-config",
				Type: "delete-config",
				Inputs: v1alpha1.StepInputs{
					{
						From:         "configName",
						ParameterKey: "name",
					},
				},
			},
			}}),
			Mode:    workflow.Mode,
			SubMode: workflow.SubMode,
			Default: &defaultW,
		}
		updateRes := put(fmt.Sprintf("/applications/%s/workflows/%s", appName, workflow.Name), uwr)

		var workflowBase apisv1.WorkflowBase
		Expect(decodeResponseBody(updateRes, &workflowBase)).Should(Succeed())
		Expect(cmp.Diff(workflowBase.Alias, uwr.Alias)).Should(BeEmpty())
		Expect(cmp.Diff(workflowBase.Description, uwr.Description)).Should(BeEmpty())
		Expect(cmp.Diff(len(workflowBase.Steps), 2)).Should(BeEmpty())
	})

	It("Run the workflow", func() {
		req := apisv1.ApplicationDeployRequest{
			WorkflowName: repository.ConvertWorkflowName(envName),
			TriggerType:  "web",
		}
		res := post(fmt.Sprintf("/applications/%s/deploy", appName), req)
		var adr apisv1.ApplicationDeployResponse
		Expect(decodeResponseBody(res, &adr)).Should(Succeed())
		Expect(adr.ApplicationRevisionBase.Version).ShouldNot(BeEmpty())
		Expect(adr.WorkflowRecord.Name).ShouldNot(BeEmpty())
	})

	It("Detail the record", func() {
		res := get(fmt.Sprintf("/applications/%s/workflows/%s/records", appName, repository.ConvertWorkflowName(envName)))
		var lrr apisv1.ListWorkflowRecordsResponse
		Expect(decodeResponseBody(res, &lrr)).Should(Succeed())
		Expect(lrr.Total).Should(Equal(int64(1)))

		recordName = lrr.Records[0].Name
		Eventually(func() error {
			recordRes := get(fmt.Sprintf("/applications/%s/workflows/%s/records/%s", appName, repository.ConvertWorkflowName(envName), lrr.Records[0].Name))
			var dr apisv1.DetailWorkflowRecordResponse
			if err := decodeResponseBody(recordRes, &dr); err != nil {
				return err
			}
			if dr.Status != string(v1alpha1.WorkflowStateSucceeded) {
				return fmt.Errorf("the workflow status is %s, not succeeded.", dr.Status)
			}
			return nil
		}).WithTimeout(time.Minute * 1).WithPolling(3 * time.Second).Should(BeNil())
	})

	It("Load the step inputs", func() {
		res := get(fmt.Sprintf("/applications/%s/workflows/%s/records/%s/inputs?step=delete-config", appName, repository.ConvertWorkflowName(envName), recordName))
		var ir apisv1.GetPipelineRunInputResponse
		Expect(decodeResponseBody(res, &ir)).Should(Succeed())
		Expect(len(ir.StepInputs)).Should(Equal(1))
		Expect(len(ir.StepInputs[0].Values)).Should(Equal(1))
		Expect(ir.StepInputs[0].Values[0].Value).Should(Equal("\"demo\"\n"))
	})

	It("Load the step outputs", func() {
		res := get(fmt.Sprintf("/applications/%s/workflows/%s/records/%s/outputs?step=read-config", appName, repository.ConvertWorkflowName(envName), recordName))
		var or apisv1.GetPipelineRunOutputResponse
		Expect(decodeResponseBody(res, &or)).Should(Succeed())
		Expect(len(or.StepOutputs)).Should(Equal(1))
		Expect(len(or.StepOutputs[0].Values)).Should(Equal(1))
		Expect(or.StepOutputs[0].Values[0].Value).Should(Equal("\"demo\"\n"))
	})

})
