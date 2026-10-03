/*
Copyright 2026 The KubeVela Authors.

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

package cli

import (
	"bytes"
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test the commands of auth", func() {
	var f cmd.Factory
	BeforeEach(func() {
		f = cmd.NewTestFactory(cfg, k8sClient)
	})

	It("Test revoke-privileges removes what grant-privileges granted", func() {
		buffer := bytes.NewBuffer(nil)
		streams := util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer}
		bindingKey := apitypes.NamespacedName{Name: auth.KubeVelaWriterRoleName + ":binding"}

		grantCmd := AuthCommandGroup(f, "", streams)
		grantCmd.SetArgs([]string{"grant-privileges", "--user", "auth-test-user"})
		Expect(grantCmd.Execute()).Should(Succeed())

		binding := &rbacv1.ClusterRoleBinding{}
		Expect(k8sClient.Get(context.Background(), bindingKey, binding)).Should(Succeed())
		Expect(binding.Subjects).Should(HaveLen(1))
		Expect(binding.Subjects[0].Name).Should(Equal("auth-test-user"))

		revokeCmd := AuthCommandGroup(f, "", streams)
		revokeCmd.SetArgs([]string{"revoke-privileges", "--user", "auth-test-user"})
		Expect(revokeCmd.Execute()).Should(Succeed())

		err := k8sClient.Get(context.Background(), bindingKey, &rbacv1.ClusterRoleBinding{})
		Expect(kerrors.IsNotFound(err)).Should(BeTrue())
	})

	It("Test revoke-privileges only removes the matching subject, keeping the rest", func() {
		buffer := bytes.NewBuffer(nil)
		streams := util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer}
		bindingKey := apitypes.NamespacedName{Name: auth.KubeVelaWriterRoleName + ":binding"}

		grantCmd := AuthCommandGroup(f, "", streams)
		grantCmd.SetArgs([]string{"grant-privileges", "--user", "auth-test-user-a"})
		Expect(grantCmd.Execute()).Should(Succeed())

		grantCmd = AuthCommandGroup(f, "", streams)
		grantCmd.SetArgs([]string{"grant-privileges", "--user", "auth-test-user-b"})
		Expect(grantCmd.Execute()).Should(Succeed())

		revokeCmd := AuthCommandGroup(f, "", streams)
		revokeCmd.SetArgs([]string{"revoke-privileges", "--user", "auth-test-user-a"})
		Expect(revokeCmd.Execute()).Should(Succeed())

		binding := &rbacv1.ClusterRoleBinding{}
		Expect(k8sClient.Get(context.Background(), bindingKey, binding)).Should(Succeed())
		Expect(binding.Subjects).Should(HaveLen(1))
		Expect(binding.Subjects[0].Name).Should(Equal("auth-test-user-b"))

		revokeCmd = AuthCommandGroup(f, "", streams)
		revokeCmd.SetArgs([]string{"revoke-privileges", "--user", "auth-test-user-b"})
		Expect(revokeCmd.Execute()).Should(Succeed())
	})
})
