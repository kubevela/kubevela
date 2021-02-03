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

package cli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestEnvCommandPersistentPreRunE(t *testing.T) {
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := common.Args{}
	cmd := NewEnvListCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
	cmd = NewEnvInitCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
	cmd = NewEnvSetCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
	cmd = NewEnvDeleteCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

var _ = Describe("Test ENV ", func() {
	It("make env crud test", func() {
		ctx := context.Background()

		By("Create Default Env")
		err := env.InitDefaultEnv(ctx, k8sClient)
		Expect(err).ToNot(HaveOccurred())

		By("check and compare create default env success")
		curEnv, err := env.GetCurrentEnv(ctx, k8sClient)
		Expect(err).ToNot(HaveOccurred())
		Expect(curEnv.Name).Should(Equal("default"))
		envMeta, err := GetEnv(ctx, k8sClient, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(envMeta).Should(Equal(&types.EnvMeta{
			Namespace: "default",
			Name:      "default",
			Current:   "*",
		}))

		io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
		exp := &types.EnvMeta{
			Namespace: "test1",
			Name:      "env1",
			Current:   "*",
		}

		By("Create env1")
		err = CreateOrUpdateEnv(ctx, k8sClient, exp, []string{"env1"}, io)
		Expect(err).ToNot(HaveOccurred())

		By("check and compare create env success")
		curEnv, err = env.GetCurrentEnv(ctx, k8sClient)
		Expect(err).ToNot(HaveOccurred())
		Expect(curEnv.Name).Should(Equal("env1"))
		gotEnv, err := GetEnv(ctx, k8sClient, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(gotEnv).Should(Equal(exp))

		By(" List all env")
		var b bytes.Buffer
		io.Out = &b
		err = ListEnvs(ctx, k8sClient, []string{}, io)
		Expect(err).ToNot(HaveOccurred())
		Expect(b.String()).Should(Equal("NAME   \tCURRENT\tNAMESPACE\tEMAIL\tDOMAIN\ndefault\t       \tdefault  \t     \t      \nenv1   \t*      \ttest1    \t     \t      \n"))
		b.Reset()
		err = ListEnvs(ctx, k8sClient, []string{"env1"}, io)
		Expect(err).ToNot(HaveOccurred())
		Expect(b.String()).Should(Equal("NAME\tCURRENT\tNAMESPACE\tEMAIL\tDOMAIN\nenv1\t*      \ttest1    \t     \t      \n"))
		io.Out = os.Stdout

		By("can not delete current env")
		err = DeleteEnv(ctx, k8sClient, []string{"env1"}, io)
		Expect(err).To(HaveOccurred())

		By("set as default env")
		err = SetEnv(ctx, k8sClient, []string{"default"}, io)
		Expect(err).ToNot(HaveOccurred())

		By("check env set success")
		gotEnv, err = GetEnv(ctx, k8sClient, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(gotEnv).Should(Equal(&types.EnvMeta{
			Namespace: "default",
			Name:      "default",
			Current:   "*",
		}))

		By("delete env")
		err = DeleteEnv(ctx, k8sClient, []string{"env1"}, io)
		Expect(err).ToNot(HaveOccurred())

		By("can not set as a non-exist env")
		err = SetEnv(ctx, k8sClient, []string{"env1"}, io)
		Expect(err).To(HaveOccurred())

		By("set success")
		err = SetEnv(ctx, k8sClient, []string{"default"}, io)
		Expect(err).ToNot(HaveOccurred())
	})
})

func TestEnvInitCommandPersistentPreRunE(t *testing.T) {
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := common.Args{}
	cmd := NewEnvInitCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}
