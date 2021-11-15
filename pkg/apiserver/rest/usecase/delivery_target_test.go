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
	)
	BeforeEach(func() {
		deliveryTargetUsecase = &deliveryTargetUsecaseImpl{ds: ds}
	})
	It("Test CreateDeliveryTarget function", func() {
		req := apisv1.CreateDeliveryTargetRequest{
			Name:        "test-delivery-target",
			Namespace:   "test-namespace",
			Alias:       "test-alias",
			Description: "this is a deliveryTarget",
			Kubernetes:  &apisv1.KubernetesTarget{ClusterName: "cluster-dev", Namespace: "dev"},
			Cloud:       &apisv1.CloudTarget{TerraformProviderName: "provider", Region: "us-1"},
		}
		base, err := deliveryTargetUsecase.CreateDeliveryTarget(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Name, req.Name)).Should(BeEmpty())

	})

	It("Test GetDeliveryTarget function", func() {
		deliveryTarget, err := deliveryTargetUsecase.GetDeliveryTarget(context.TODO(), "test-delivery-target")
		Expect(err).Should(BeNil())
		Expect(deliveryTarget).ShouldNot(BeNil())
		Expect(cmp.Diff(deliveryTarget.Name, "test-delivery-target")).Should(BeEmpty())
	})

	It("Test ListDeliveryTargets function", func() {
		_, err := deliveryTargetUsecase.ListDeliveryTargets(context.TODO(), 1, 1)
		Expect(err).Should(BeNil())
	})

	It("Test DetailDeliveryTarget function", func() {
		detail, err := deliveryTargetUsecase.DetailDeliveryTarget(context.TODO(),
			&model.DeliveryTarget{
				Name:        "test-delivery-target",
				Namespace:   "test-namespace",
				Alias:       "test-alias",
				Description: "this is a deliveryTarget",
				Kubernetes:  &model.KubernetesTarget{ClusterName: "cluster-dev", Namespace: "dev"},
				Cloud:       &model.CloudTarget{TerraformProviderName: "provider", Region: "us-1"}})
		Expect(err).Should(BeNil())
		Expect(detail.Name).Should(Equal("test-delivery-target"))
	})
})
