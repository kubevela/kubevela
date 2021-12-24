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
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test env usecase functions", func() {
	var (
		envUsecase *envUsecaseImpl
	)
	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "env-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		envUsecase = &envUsecaseImpl{kubeClient: k8sClient, ds: ds}
	})
	It("Test Create/Get/Delete Env function", func() {
		req := apisv1.CreateEnvRequest{
			Name:        "test-env",
			Description: "this is a env description",
		}
		base, err := envUsecase.CreateEnv(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(base.Namespace, req.Name)).Should(BeEmpty())

		By("test specified namespace to create env")
		req2 := apisv1.CreateEnvRequest{
			Name:        "test-env-2",
			Description: "this is a env description",
			Namespace:   base.Namespace,
		}
		_, err = envUsecase.CreateEnv(context.TODO(), req2)
		equal := cmp.Equal(err, bcode.ErrEnvNamespaceAlreadyBound, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())

		req3 := apisv1.CreateEnvRequest{
			Name:        "test-env-2",
			Description: "this is a env description",
			Namespace:   "default",
		}
		base, err = envUsecase.CreateEnv(context.TODO(), req3)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Namespace, "default")).Should(BeEmpty())
		var namespace corev1.Namespace
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: base.Namespace}, &namespace)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(namespace.Labels[oam.LabelNamespaceOfEnvName], req3.Name)).Should(BeEmpty())

		// clean up the env
		err = envUsecase.DeleteEnv(context.TODO(), "test-env")
		Expect(err).Should(BeNil())
		err = envUsecase.DeleteEnv(context.TODO(), "test-env-2")
		Expect(err).Should(BeNil())

		By("Test ListEnvs function")
		_, err = envUsecase.ListEnvs(context.TODO(), 1, 1, apisv1.ListEnvOptions{})
		Expect(err).Should(BeNil())
	})
})
