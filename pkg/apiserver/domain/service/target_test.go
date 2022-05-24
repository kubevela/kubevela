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

package service

import (
	"context"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

var _ = Describe("Test target service functions", func() {
	var (
		targetService  *targetServiceImpl
		projectService *projectServiceImpl
		testProject    = "target-project"
	)
	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "target-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacService := &rbacServiceImpl{Store: ds}
		projectService = &projectServiceImpl{Store: ds, K8sClient: k8sClient, RbacService: rbacService}
		targetService = &targetServiceImpl{Store: ds, K8sClient: k8sClient}
	})
	It("Test CreateTarget function", func() {
		_, err := projectService.CreateProject(context.TODO(), apisv1.CreateProjectRequest{Name: testProject})
		Expect(err).Should(BeNil())

		req := apisv1.CreateTargetRequest{
			Name:        "test--target",
			Alias:       "test-alias",
			Description: "this is a Target",
			Project:     testProject,
			Cluster:     &apisv1.ClusterTarget{ClusterName: "cluster-dev", Namespace: "dev"},
			Variable:    map[string]interface{}{"terraform-provider": "provider", "region": "us-1"},
		}
		base, err := targetService.CreateTarget(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		Expect(targetService.Store.Add(context.TODO(), &model.Cluster{Name: "cluster-dev", Alias: "dev-alias"})).Should(Succeed())

		By("Test GetTarget function")
		Target, err := targetService.GetTarget(context.TODO(), "test--target")
		Expect(err).Should(BeNil())
		Expect(Target).ShouldNot(BeNil())
		Expect(cmp.Diff(Target.Name, "test--target")).Should(BeEmpty())

		By("Test ListTargets function")
		resp, err := targetService.ListTargets(context.TODO(), 1, 1, "")
		Expect(err).Should(BeNil())
		Expect(resp.Targets[0].ClusterAlias).Should(Equal("dev-alias"))

		By("Test DetailTarget function")
		detail, err := targetService.DetailTarget(context.TODO(),
			&model.Target{
				Name:        "test--target",
				Alias:       "test-alias",
				Description: "this is a Target",
				Cluster:     &model.ClusterTarget{ClusterName: "cluster-dev", Namespace: "dev"},
				Variable:    map[string]interface{}{"terraform-provider": "provider", "region": "us-1"}})
		Expect(err).Should(BeNil())
		Expect(detail.Name).Should(Equal("test--target"))

		By("Test Delete target")
		err = targetService.DeleteTarget(context.TODO(), "test--target")
		Expect(err).Should(BeNil())
	})
})
