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

	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test delivery target usecase functions", func() {
	var (
		deliveryTargetUsecase *deliveryTargetUsecaseImpl
		projectUsecase        *projectUsecaseImpl
		testProject           = "target-project"
	)
	BeforeEach(func() {
		projectUsecase = &projectUsecaseImpl{ds: ds, kubeClient: k8sClient}
		deliveryTargetUsecase = &deliveryTargetUsecaseImpl{ds: ds, projectUsecase: projectUsecase}
	})
	It("Test CreateDeliveryTarget function", func() {
		_, err := projectUsecase.CreateProject(context.TODO(), apisv1.CreateProjectRequest{Name: testProject})
		Expect(err).Should(BeNil())

		req := apisv1.CreateDeliveryTargetRequest{
			Name:        "test-delivery-target",
			Project:     testProject,
			Alias:       "test-alias",
			Description: "this is a deliveryTarget",
			Cluster:     &apisv1.ClusterTarget{ClusterName: "cluster-dev", Namespace: "dev"},
			Variable:    map[string]interface{}{"terraform-provider": "provider", "region": "us-1"},
		}
		base, err := deliveryTargetUsecase.CreateDeliveryTarget(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

		Expect(deliveryTargetUsecase.ds.Add(context.TODO(), &model.Cluster{Name: "cluster-dev", Alias: "dev-alias"})).Should(Succeed())
	})

	It("Test GetDeliveryTarget function", func() {
		deliveryTarget, err := deliveryTargetUsecase.GetDeliveryTarget(context.TODO(), "test-delivery-target")
		Expect(err).Should(BeNil())
		Expect(deliveryTarget).ShouldNot(BeNil())
		Expect(cmp.Diff(deliveryTarget.Name, "test-delivery-target")).Should(BeEmpty())
	})

	It("Test ListDeliveryTargets function", func() {
		resp, err := deliveryTargetUsecase.ListDeliveryTargets(context.TODO(), 1, 1, "")
		Expect(err).Should(BeNil())
		Expect(resp.Targets[0].ClusterAlias).Should(Equal("dev-alias"))
	})

	It("Test DetailDeliveryTarget function", func() {
		detail, err := deliveryTargetUsecase.DetailDeliveryTarget(context.TODO(),
			&model.DeliveryTarget{
				Name:        "test-delivery-target",
				Namespace:   "test-namespace",
				Alias:       "test-alias",
				Description: "this is a deliveryTarget",
				Cluster:     &model.ClusterTarget{ClusterName: "cluster-dev", Namespace: "dev"},
				Variable:    map[string]interface{}{"terraform-provider": "provider", "region": "us-1"}})
		Expect(err).Should(BeNil())
		Expect(detail.Name).Should(Equal("test-delivery-target"))
	})
})
