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

package usecase

import (
	"context"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test project usecase functions", func() {
	var (
		projectUsecase *projectUsecaseImpl
	)
	BeforeEach(func() {
		projectUsecase = &projectUsecaseImpl{k8sClient: k8sClient, ds: ds}
	})
	It("Test Create project function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		base, err := projectUsecase.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())
		_, err = projectUsecase.ListProjects(context.TODO())
		Expect(err).Should(BeNil())
		projectUsecase.DeleteProject(context.TODO(), "test-project")
	})
	It("Test project initialize function", func() {
		projectUsecase.initDefaultProjectEnvTarget()

		By("test env created")
		var namespace corev1.Namespace
		err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: DefaultInitNamespace}, &namespace)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(namespace.Labels[oam.LabelNamespaceOfEnvName], DefaultInitName)).Should(BeEmpty())
		Expect(cmp.Diff(namespace.Labels[oam.LabelNamespaceOfTargetName], DefaultInitName)).Should(BeEmpty())
		Expect(cmp.Diff(namespace.Labels[oam.LabelControlPlaneNamespaceUsage], oam.VelaNamespaceUsageEnv)).Should(BeEmpty())
		Expect(cmp.Diff(namespace.Labels[oam.LabelRuntimeNamespaceUsage], oam.VelaNamespaceUsageTarget)).Should(BeEmpty())

		By("check project created")
		dp, err := projectUsecase.GetProject(context.TODO(), DefaultInitName)
		Expect(err).Should(BeNil())
		Expect(dp.Alias).Should(BeEquivalentTo("Default"))
		Expect(dp.Description).Should(BeEquivalentTo(DefaultProjectDescription))

		By("check env created")
		envImpl := &envUsecaseImpl{kubeClient: k8sClient, ds: ds}
		env, err := envImpl.GetEnv(context.TODO(), DefaultInitName)
		Expect(err).Should(BeNil())
		Expect(env.Alias).Should(BeEquivalentTo("Default"))
		Expect(env.Description).Should(BeEquivalentTo(DefaultEnvDescription))
		Expect(env.Project).Should(BeEquivalentTo(DefaultInitName))
		Expect(env.Targets).Should(BeEquivalentTo([]string{DefaultInitName}))
		Expect(env.Namespace).Should(BeEquivalentTo(DefaultInitNamespace))

		By("check target created")
		targetImpl := &targetUsecaseImpl{k8sClient: k8sClient, ds: ds}
		tg, err := targetImpl.GetTarget(context.TODO(), DefaultInitName)
		Expect(err).Should(BeNil())
		Expect(tg.Alias).Should(BeEquivalentTo("Default"))
		Expect(tg.Description).Should(BeEquivalentTo(DefaultTargetDescription))
		Expect(tg.Cluster).Should(BeEquivalentTo(&model.ClusterTarget{
			ClusterName: multicluster.ClusterLocalName,
			Namespace:   DefaultInitNamespace,
		}))
		Expect(env.Targets).Should(BeEquivalentTo([]string{DefaultInitName}))
		Expect(env.Namespace).Should(BeEquivalentTo(DefaultInitNamespace))
	})

})
