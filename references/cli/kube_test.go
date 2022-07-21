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

package cli

import (
	"bytes"
	"context"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test kube apply cli", func() {

	When("test vela kube apply", func() {

		It("should not have err and applied all objects", func() {

			buffer := bytes.NewBuffer(nil)
			ioStreams := util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer}

			o := &KubeApplyOptions{}
			o.IOStreams = ioStreams
			o.files = []string{"./test-data/kubeapply"}
			ctx := context.Background()
			Expect(o.Complete(ctx)).Should(BeNil())
			Expect(o.Validate()).Should(BeNil())
			Expect(o.Run(ctx, k8sClient)).Should(BeNil())

			By("test kube apply in dry-run mod")
			o.dryRun = true
			Expect(o.Run(ctx, k8sClient)).Should(BeNil())
			buf, ok := ioStreams.Out.(*bytes.Buffer)
			Expect(ok).Should(BeTrue())
			Expect(strings.Contains(buf.String(), "error")).Should(BeFalse())

			By("test kube apply in different namespace, new namespace")
			var newns = "test-kube-apply"
			err := k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: v1.ObjectMeta{Name: newns},
			})
			Expect(err).Should(BeNil())

			By("apply objects")
			o = &KubeApplyOptions{}
			o.IOStreams = ioStreams
			o.files = []string{"./test-data/kubeapply"}
			o.namespace = newns
			Expect(o.Complete(ctx)).Should(BeNil())
			Expect(o.Validate()).Should(BeNil())
			Expect(o.Run(ctx, k8sClient)).Should(BeNil())

			By("check objects configmap created")
			cml := corev1.ConfigMapList{}
			Expect(k8sClient.List(ctx, &cml, client.InNamespace(newns))).Should(BeNil())
			Expect(len(cml.Items)).Should(BeEquivalentTo(3))

		})
	})
})
