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
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test env service functions", func() {
	var (
		envService *envServiceImpl
		ds         datastore.DataStore
	)
	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "env-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		rbacService := &rbacServiceImpl{Store: ds}
		projectService := &projectServiceImpl{Store: ds, K8sClient: k8sClient, RbacService: rbacService}
		envService = &envServiceImpl{KubeClient: k8sClient, Store: ds, ProjectService: projectService}
	})
	It("Test Create/Get/Delete Env function", func() {
		// create target
		err := ds.Add(context.TODO(), &model.Target{Name: "env-test"})
		Expect(err).Should(BeNil())
		err = ds.Add(context.TODO(), &model.Target{Name: "env-test-2"})
		Expect(err).Should(BeNil())

		req := apisv1.CreateEnvRequest{
			Name:        "test-env",
			Description: "this is a env description",
		}
		base, err := envService.CreateEnv(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(base.Namespace, req.Name)).Should(BeEmpty())

		By("test specified namespace to create env")
		req2 := apisv1.CreateEnvRequest{
			Name:        "test-env-2",
			Description: "this is a env description",
			Namespace:   base.Namespace,
		}
		_, err = envService.CreateEnv(context.TODO(), req2)
		equal := cmp.Equal(err, bcode.ErrEnvNamespaceAlreadyBound, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())

		req3 := apisv1.CreateEnvRequest{
			Name:        "test-env-2",
			Description: "this is a env description",
			Namespace:   "test-env-22",
			Project:     "env-project",
			Targets:     []string{"env-test"},
		}
		base, err = envService.CreateEnv(context.TODO(), req3)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Namespace, req3.Namespace)).Should(BeEmpty())
		var namespace corev1.Namespace
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: base.Namespace}, &namespace)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(namespace.Labels[oam.LabelNamespaceOfEnvName], req3.Name)).Should(BeEmpty())

		var roleBinding rbacv1.RoleBinding
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: auth.KubeVelaWriterAppRoleName + ":binding", Namespace: base.Namespace}, &roleBinding)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(roleBinding.RoleRef.Name, auth.KubeVelaWriterAppRoleName)).Should(BeEmpty())

		// test env target conflict
		req4 := apisv1.CreateEnvRequest{
			Name:        "test-env-3",
			Description: "this is a env description",
			Namespace:   "test-env-22",
			Project:     "env-project",
			Targets:     []string{"env-test"},
		}
		_, err = envService.CreateEnv(context.TODO(), req4)
		Expect(cmp.Equal(err, bcode.ErrEnvTargetConflict, cmpopts.EquateErrors())).Should(BeTrue())

		// test update env
		req5 := apisv1.UpdateEnvRequest{
			Description: "this is a env description update",
			Targets:     []string{"env-test"},
		}
		env, err := envService.UpdateEnv(context.TODO(), "test-env-2", req5)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(env.Description, req5.Description)).Should(BeEmpty())

		By("Test update the targets of the env")
		req6 := apisv1.UpdateEnvRequest{
			Description: "this is a env description update",
			Targets:     []string{"env-test", "env-test-2"},
		}
		env, err = envService.UpdateEnv(context.TODO(), "test-env-2", req6)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(env.Targets), len(req6.Targets))).Should(BeEmpty())

		Expect(k8sClient.Create(context.TODO(), &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "env-app",
				Namespace: env.Namespace,
				Labels: map[string]string{
					velatypes.LabelSourceOfTruth: velatypes.FromUX,
				},
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{},
			},
		})).Should(BeNil())

		req7 := apisv1.UpdateEnvRequest{
			Description: "this is a env description update",
			Targets:     []string{"env-test"},
		}
		_, err = envService.UpdateEnv(context.TODO(), "test-env-2", req7)
		Expect(err).Should(Equal(bcode.ErrEnvTargetNotAllowDelete))

		// clean up the env
		err = envService.DeleteEnv(context.TODO(), "test-env")
		Expect(err).Should(BeNil())
		err = envService.DeleteEnv(context.TODO(), "test-env-2")
		Expect(err).Should(BeNil())

		By("Test ListEnvs function")
		_, err = envService.ListEnvs(context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "admin"), 1, 1, apisv1.ListEnvOptions{})
		Expect(err).Should(BeNil())
	})

	It("test checkEqual", func() {
		Expect(checkEqual([]string{"default"}, []string{"default", "dev"})).Should(BeFalse())
		Expect(checkEqual([]string{"default"}, []string{"default"})).Should(BeTrue())
	})
})
