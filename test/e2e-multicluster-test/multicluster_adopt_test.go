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

package e2e_multicluster_test

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/rand"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Test adopt commands", func() {

	Context("Test vela adopt commands", func() {

		var ns string

		BeforeEach(func() {
			ns = "test-adopt-" + rand.RandomString(4)
			Expect(k8s.EnsureNamespace(context.Background(), k8sClient, ns)).Should(Succeed())
		})

		AfterEach(func() {
			Expect(k8s.ClearNamespace(context.Background(), k8sClient, ns)).Should(Succeed())
		})

		It("Test vela adopt native resources with read-only mode", func() {
			ctx := context.Background()
			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "adopt-cm", Namespace: ns}})).Should(Succeed())
			Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "adopt-secret", Namespace: ns}})).Should(Succeed())
			_, err := execCommand("adopt", "configmap/adopt-cm", "secret/adopt-secret", "--app-name=adopt-test", "-n="+ns, "--apply")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "adopt-test", Namespace: ns}, app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "adopt-cm", Namespace: ns}, &corev1.ConfigMap{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "adopt-cm", Namespace: ns}, &corev1.ConfigMap{})).Should(Succeed())
		})

		It("Test vela adopt helm chart with take-over mode", func() {
			ctx, fn := context.WithTimeout(context.Background(), time.Second*60)
			defer fn()
			bs, err := exec.CommandContext(ctx, "helm", "install", "vela-test", "./testdata/chart/test", "-n", ns).CombinedOutput()
			_, _ = fmt.Fprintf(GinkgoWriter, "%s\n", string(bs))
			Expect(err).Should(Succeed())
			_, err = execCommand("adopt", "vela-test", "--mode=take-over", "--type=helm", "-n="+ns, "--apply", "--recycle")
			Expect(err).Should(Succeed())
			app := &v1beta1.Application{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "vela-test", Namespace: ns}, app)).Should(Succeed())
				g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning))
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "vela-test", Namespace: ns}, &corev1.ConfigMap{})).Should(Succeed())
			}).WithTimeout(20 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(errors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: "vela-test", Namespace: ns}, &corev1.ConfigMap{}))).Should(BeTrue())
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})
	})

})
