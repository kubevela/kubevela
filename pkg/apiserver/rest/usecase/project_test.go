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
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test project usecase functions", func() {
	var (
		projectUsecase *projectUsecaseImpl
	)
	BeforeEach(func() {
		projectUsecase = &projectUsecaseImpl{kubeClient: k8sClient, ds: ds}
	})
	It("Test Createproject function", func() {
		req := apisv1.CreateProjectRequest{
			Name:        "test-project",
			Description: "this is a project description",
		}
		base, err := projectUsecase.CreateProject(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(base.Namespace, fmt.Sprintf("project-%s", req.Name))).Should(BeEmpty())

		By("test specified namespace to create project")
		req2 := apisv1.CreateProjectRequest{
			Name:        "test-project-2",
			Description: "this is a project description",
			Namespace:   base.Namespace,
		}
		_, err = projectUsecase.CreateProject(context.TODO(), req2)
		equal := cmp.Equal(err, bcode.ErrProjectNamespaceIsExist, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())

		req3 := apisv1.CreateProjectRequest{
			Name:        "test-project-2",
			Description: "this is a project description",
			Namespace:   "default",
		}
		base, err = projectUsecase.CreateProject(context.TODO(), req3)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Namespace, "default")).Should(BeEmpty())
		var namespace corev1.Namespace
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: base.Namespace}, &namespace)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(namespace.Labels[oam.LabelProjectNamesapce], req3.Name)).Should(BeEmpty())
	})

	It("Test ListProject function", func() {
		_, err := projectUsecase.ListProjects(context.TODO())
		Expect(err).Should(BeNil())
	})
})
