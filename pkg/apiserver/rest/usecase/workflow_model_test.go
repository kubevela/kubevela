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

package usecase

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

func TestCompareWorkflowSteps(t *testing.T) {
	existSteps := []model.WorkflowStep{
		{
			Name: "step1",
			Type: "deploy2env",
			Properties: &model.JSONStruct{
				"policy": "env-policy",
				"env":    "target1",
			},
		},
		{
			Name: "suspend",
			Type: "suspend",
		},
		{
			Name: "step2",
			Type: "deploy2env",
			Properties: &model.JSONStruct{
				"policy": "env-policy",
				"env":    "target2",
			},
		},
		{
			Name: "step3",
			Type: "deploy2env",
			Properties: &model.JSONStruct{
				"policy": "env-policy",
				"env":    "target3",
			},
		},
		{
			Name:       "notify",
			Type:       "notify",
			Properties: &model.JSONStruct{"message": "dddd"},
		},
	}
	newSteps := []model.WorkflowStep{
		{
			Name:       "step1",
			Type:       "deploy",
			Properties: &model.JSONStruct{"policies": []string{"target1"}},
		},
		{
			Name:       "step2",
			Type:       "deploy",
			Properties: &model.JSONStruct{"policies": []string{"target2"}},
		},
		{
			Name:       "step4",
			Type:       "deploy",
			Properties: &model.JSONStruct{"policies": []string{"target4"}},
		},
	}
	exist := createWorkflowSteps(existSteps, []datastore.Entity{
		&model.ApplicationPolicy{
			Name: "env-policy",
			Type: "env-binding",
			Properties: &model.JSONStruct{
				"envs": []map[string]interface{}{
					{
						"name": "target1",
						"placement": map[string]interface{}{
							"clusterSelector": map[string]interface{}{
								"name": "cluster1",
							},
							"namespaceSelector": map[string]interface{}{
								"name": "ns1",
							},
						},
					},
					{
						"name": "target2",
						"placement": map[string]interface{}{
							"clusterSelector": map[string]interface{}{
								"name": "cluster2",
							},
							"namespaceSelector": map[string]interface{}{
								"name": "ns2",
							},
						},
					},
					{
						"name": "target3",
						"placement": map[string]interface{}{
							"clusterSelector": map[string]interface{}{
								"name": "cluster3",
							},
							"namespaceSelector": map[string]interface{}{
								"name": "ns3",
							},
						},
					},
				},
			},
		},
	})
	new := createWorkflowSteps(newSteps, []datastore.Entity{
		&model.ApplicationPolicy{
			Name: "target1",
			Type: "topology",
			Properties: &model.JSONStruct{
				"clusters":  []string{"cluster1"},
				"namespace": "ns1",
			},
		},
		&model.ApplicationPolicy{
			Name: "target2",
			Type: "topology",
			Properties: &model.JSONStruct{
				"clusters":  []string{"cluster2"},
				"namespace": "ns2",
			},
		},
		&model.ApplicationPolicy{
			Name: "target4",
			Type: "topology",
			Properties: &model.JSONStruct{
				"clusters":  []string{"cluster4"},
				"namespace": "ns4",
			},
		},
	})
	assert.Equal(t, len(exist), 5)
	assert.Equal(t, len(new), 3)
	result := compareWorkflowSteps(exist, new)
	t.Log(result.String())
	assert.Equal(t, len(result), 6)
	assert.Equal(t, result[0].state, keepState)
	assert.Equal(t, result[1].state, keepState)
	assert.Equal(t, result[3].state, deleteState)
	assert.Equal(t, result[5].state, newState)
	assert.Equal(t, result[5].stepType, "deploy")
	workflowReadySteps := result.getSteps(newSteps, existSteps)
	assert.Equal(t, len(workflowReadySteps), 5)
}

var _ = Describe("Test workflow model", func() {
	var ds datastore.DataStore
	var (
		envUsecase        *envUsecaseImpl
		envBindingUsecase *envBindingUsecaseImpl
		workflowUsecase   *workflowUsecaseImpl
		definitionUsecase DefinitionUsecase
	)
	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "workflow-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacUsecase := &rbacUsecaseImpl{ds: ds}
		projectUsecase := &projectUsecaseImpl{ds: ds, k8sClient: k8sClient, rbacUsecase: rbacUsecase}
		envUsecase = &envUsecaseImpl{ds: ds, kubeClient: k8sClient, projectUsecase: projectUsecase}
		workflowUsecase = &workflowUsecaseImpl{ds: ds, kubeClient: k8sClient, envUsecase: envUsecase}
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient, caches: utils.NewMemoryCacheStore(context.TODO())}
		envBindingUsecase = &envBindingUsecaseImpl{ds: ds, workflowUsecase: workflowUsecase, definitionUsecase: definitionUsecase, kubeClient: k8sClient, envUsecase: envUsecase}

	})
	It("update the workflow after added a cloud component", func() {
		definition := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aliyun-rds",
				Namespace: types.DefaultKubeVelaNS,
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{
					Type: TerraformWorkloadType,
				},
			},
		}
		err := k8sClient.Create(context.TODO(), definition)
		Expect(err).Should(BeNil())

		app := &model.Application{
			Name:    "test-mixture-components",
			Project: "default",
		}
		webComponent := &model.ApplicationComponent{
			Name:          "web",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "webservice",
		}
		env := &model.Env{
			Name:    "dev",
			Project: "default",
			Targets: []string{"target1", "target2"},
		}
		target1 := &model.Target{
			Name:    "target1",
			Project: "default",
			Cluster: &model.ClusterTarget{ClusterName: "local", Namespace: "target1"},
		}
		target2 := &model.Target{
			Name:    "target2",
			Project: "default",
			Cluster: &model.ClusterTarget{ClusterName: "local", Namespace: "target2"},
		}
		err = ds.BatchAdd(context.TODO(), []datastore.Entity{app, webComponent, target1, target2, env})
		Expect(err).Should(BeNil())

		err = envBindingUsecase.createEnvWorkflow(context.TODO(), app, env, true)
		Expect(err).Should(BeNil())

		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(2))

		cloudComponent := &model.ApplicationComponent{
			Name:          "cloud",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "aliyun-rds",
		}

		err = ds.BatchAdd(context.TODO(), []datastore.Entity{cloudComponent})
		Expect(err).Should(BeNil())

		err = UpdateEnvWorkflow(context.Background(), k8sClient, ds, app, env)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(4))
		Expect(workflow.Steps[0].Type).Should(Equal(DeployCloudResource))

		entities, err := ds.List(context.TODO(), &model.ApplicationPolicy{AppPrimaryKey: app.PrimaryKey()}, &datastore.ListOptions{
			FilterOptions: datastore.FilterOptions{
				In: []datastore.InQueryOption{{
					Key:    "type",
					Values: []string{v1alpha1.TopologyPolicyType, v1alpha1.EnvBindingPolicyType},
				}},
			},
		})
		Expect(err).Should(BeNil())
		Expect(len(entities)).Should(Equal(3))
		Expect(entities[0].(*model.ApplicationPolicy).Name).Should(Equal("env-bindings-dev"))

		By("test the case that delete the cloud component")
		err = ds.Delete(context.TODO(), cloudComponent)
		Expect(err).Should(BeNil())
		err = UpdateEnvWorkflow(context.Background(), k8sClient, ds, app, env)
		Expect(err).Should(BeNil())
		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(2))
	})

	It("update the workflow after added a common component", func() {

		app := &model.Application{
			Name:    "test-mixture-components-2",
			Project: "default",
		}
		cloudComponent := &model.ApplicationComponent{
			Name:          "cloud",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "aliyun-rds",
		}

		env := &model.Env{
			Name:    "dev",
			Project: "default",
			Targets: []string{"target1", "target2"},
		}

		err := ds.BatchAdd(context.TODO(), []datastore.Entity{app, cloudComponent})
		Expect(err).Should(BeNil())

		err = envBindingUsecase.createEnvWorkflow(context.TODO(), app, env, true)
		Expect(err).Should(BeNil())

		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(2))

		target3 := &model.Target{
			Name:    "target3",
			Project: "default",
			Cluster: &model.ClusterTarget{ClusterName: "local", Namespace: "target3"},
		}

		webComponent := &model.ApplicationComponent{
			Name:          "web",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "webservice",
		}

		env.Targets = []string{"target1", "target2", "target3"}

		err = ds.Put(context.TODO(), env)
		Expect(err).Should(BeNil())

		err = ds.BatchAdd(context.TODO(), []datastore.Entity{webComponent, target3})
		Expect(err).Should(BeNil())

		err = UpdateEnvWorkflow(context.Background(), k8sClient, ds, app, env)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(6))
		Expect(workflow.Steps[0].Type).Should(Equal(DeployCloudResource))

		entities, err := ds.List(context.TODO(), &model.ApplicationPolicy{AppPrimaryKey: app.PrimaryKey()}, &datastore.ListOptions{
			FilterOptions: datastore.FilterOptions{
				In: []datastore.InQueryOption{{
					Key:    "type",
					Values: []string{v1alpha1.TopologyPolicyType, v1alpha1.EnvBindingPolicyType},
				}},
			},
		})
		Expect(err).Should(BeNil())
		Expect(len(entities)).Should(Equal(4))
		Expect(entities[0].(*model.ApplicationPolicy).Name).Should(Equal("env-bindings-dev"))
		Expect(len((*entities[0].(*model.ApplicationPolicy).Properties)["envs"].([]interface{}))).Should(Equal(3))
	})

	It("with the custom steps", func() {
		app := &model.Application{
			Name:    "test-mixture-components-3",
			Project: "default",
		}
		webComponent := &model.ApplicationComponent{
			Name:          "web",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "webservice",
		}

		env := &model.Env{
			Name:    "dev",
			Project: "default",
			Targets: []string{"target1", "target2"},
		}

		err := ds.BatchAdd(context.TODO(), []datastore.Entity{app, webComponent})
		Expect(err).Should(BeNil())

		err = envBindingUsecase.createEnvWorkflow(context.TODO(), app, env, true)
		Expect(err).Should(BeNil())

		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())

		workflow.Steps = []model.WorkflowStep{
			workflow.Steps[0], {
				Type: "suspend",
				Name: "suspend",
			}, workflow.Steps[1], {
				Type: "notification",
				Name: "notification",
			},
		}

		err = ds.Put(context.TODO(), workflow)
		Expect(err).Should(BeNil())

		env.Targets = []string{"target1", "target2", "target3"}

		err = ds.Put(context.TODO(), env)
		Expect(err).Should(BeNil())

		err = UpdateEnvWorkflow(context.Background(), k8sClient, ds, app, env)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(5))
		Expect(workflow.Steps[1].Type).Should(Equal("suspend"))
		Expect(workflow.Steps[3].Type).Should(Equal("notification"))
	})

	It("with the concurrent steps", func() {
		app := &model.Application{
			Name:    "test-mixture-components-4",
			Project: "default",
		}
		webComponent := &model.ApplicationComponent{
			Name:          "web",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "webservice",
		}

		env := &model.Env{
			Name:    "dev",
			Project: "default",
			Targets: []string{"target1", "target2"},
		}

		err := ds.BatchAdd(context.TODO(), []datastore.Entity{app, webComponent})
		Expect(err).Should(BeNil())

		err = envBindingUsecase.createEnvWorkflow(context.TODO(), app, env, true)
		Expect(err).Should(BeNil())

		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())

		step := workflow.Steps[0]
		(*step.Properties)["policies"] = []string{"target1", "target2"}
		workflow.Steps = []model.WorkflowStep{step}

		err = ds.Put(context.TODO(), workflow)
		Expect(err).Should(BeNil())

		env.Targets = []string{"target1", "target2", "target3"}

		err = ds.Put(context.TODO(), env)
		Expect(err).Should(BeNil())

		err = UpdateEnvWorkflow(context.Background(), k8sClient, ds, app, env)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(2))
		Expect(workflow.Steps[0].Properties).ShouldNot(BeNil())
		// the concurrent step should be kept.
		Expect(len((*workflow.Steps[0].Properties)["policies"].([]interface{}))).Should(Equal(2))
		Expect((*workflow.Steps[1].Properties)["policies"].([]interface{})[0].(string)).Should(Equal("target3"))

		env.Targets = []string{"target2", "target3"}

		err = ds.Put(context.TODO(), env)
		Expect(err).Should(BeNil())

		err = UpdateEnvWorkflow(context.Background(), k8sClient, ds, app, env)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(2))
		// the concurrent step should be kept.
		Expect(workflow.Steps[0].Properties).ShouldNot(BeNil())
		Expect(len((*workflow.Steps[0].Properties)["policies"].([]interface{}))).Should(Equal(1))
		Expect((*workflow.Steps[0].Properties)["policies"].([]interface{})[0]).Should(Equal("target2"))
		Expect((*workflow.Steps[1].Properties)["policies"].([]interface{})[0].(string)).Should(Equal("target3"))

		entities, err := ds.List(context.TODO(), &model.ApplicationPolicy{AppPrimaryKey: app.PrimaryKey()}, &datastore.ListOptions{
			FilterOptions: datastore.FilterOptions{
				In: []datastore.InQueryOption{{
					Key:    "type",
					Values: []string{v1alpha1.TopologyPolicyType, v1alpha1.EnvBindingPolicyType},
				}},
			},
		})
		Expect(err).Should(BeNil())
		Expect(len(entities)).Should(Equal(2))
	})

	It("update the workflow after deleted a target", func() {

		app := &model.Application{
			Name:    "test-mixture-components-5",
			Project: "default",
		}
		cloudComponent := &model.ApplicationComponent{
			Name:          "cloud",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "aliyun-rds",
		}

		webComponent := &model.ApplicationComponent{
			Name:          "web",
			AppPrimaryKey: app.PrimaryKey(),
			Type:          "webservice",
		}

		env := &model.Env{
			Name:    "dev",
			Project: "default",
			Targets: []string{"target1", "target2"},
		}

		err := ds.BatchAdd(context.TODO(), []datastore.Entity{app, cloudComponent, webComponent})
		Expect(err).Should(BeNil())

		err = envBindingUsecase.createEnvWorkflow(context.TODO(), app, env, true)
		Expect(err).Should(BeNil())

		workflow, err := workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(4))

		env.Targets = []string{"target1"}

		err = ds.Put(context.TODO(), env)
		Expect(err).Should(BeNil())

		err = UpdateEnvWorkflow(context.Background(), k8sClient, ds, app, env)
		Expect(err).Should(BeNil())

		workflow, err = workflowUsecase.GetWorkflow(context.TODO(), app, convertWorkflowName(env.Name))
		Expect(err).Should(BeNil())
		Expect(len(workflow.Steps)).Should(Equal(2))
		Expect(workflow.Steps[0].Type).Should(Equal(DeployCloudResource))

		entities, err := ds.List(context.TODO(), &model.ApplicationPolicy{AppPrimaryKey: app.PrimaryKey()}, &datastore.ListOptions{
			FilterOptions: datastore.FilterOptions{
				In: []datastore.InQueryOption{{
					Key:    "type",
					Values: []string{v1alpha1.TopologyPolicyType, v1alpha1.EnvBindingPolicyType},
				}},
			},
		})
		Expect(err).Should(BeNil())
		Expect(len(entities)).Should(Equal(2))
	})
})
