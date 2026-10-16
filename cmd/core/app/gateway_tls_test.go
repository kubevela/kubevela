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

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/servingcert"
)

const (
	testGatewayNamespace  = "vela-system"
	testGatewaySecretName = "kubevela-vela-core-cluster-gateway-tls-v2"
)

var _ = Describe("setupClusterGatewayTLS", func() {
	var (
		ctx context.Context
		c   ctrlclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(&apiregistrationv1.APIService{
			ObjectMeta: metav1.ObjectMeta{Name: multicluster.ClusterGatewayAPIServiceName},
			Spec: apiregistrationv1.APIServiceSpec{
				Group:   "cluster.core.oam.dev",
				Version: "v1alpha1",
				Service: &apiregistrationv1.ServiceReference{
					Namespace: testGatewayNamespace,
					Name:      "kubevela-cluster-gateway-service",
					Port:      ptr.To[int32](9443),
				},
			},
		}).Build()
	})

	secretCA := func() []byte {
		secret := &corev1.Secret{}
		Expect(c.Get(ctx, ctrlclient.ObjectKey{Namespace: testGatewayNamespace, Name: testGatewaySecretName}, secret)).To(Succeed())
		return secret.Data[servingcert.CAKey]
	}

	It("writes the CA to the CA file in direct mode", func() {
		caFile := filepath.Join(GinkgoT().TempDir(), "cluster-gateway", "ca")

		bundle, err := setupClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecretName, caFile)
		Expect(err).NotTo(HaveOccurred())

		written, err := os.ReadFile(caFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(written).To(Equal(secretCA()))
		Expect(bundle).To(Equal(secretCA()))
	})

	It("only sets the APIService caBundle when requests go through the aggregator", func() {
		_, err := setupClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecretName, "")
		Expect(err).NotTo(HaveOccurred())

		apiService := &apiregistrationv1.APIService{}
		Expect(c.Get(ctx, ctrlclient.ObjectKey{Name: multicluster.ClusterGatewayAPIServiceName}, apiService)).To(Succeed())
		Expect(apiService.Spec.CABundle).To(Equal(secretCA()))
	})

	Describe("clusterGatewayTLSResync", func() {
		var restarts int32
		restart := func() { atomic.AddInt32(&restarts, 1) }

		BeforeEach(func() { restarts = 0 })

		// putExpiringJobSecret stores a Secret like an old Job-made one (no CA
		// key) whose cert is expiring, so the next pass has to replace the CA.
		putExpiringJobSecret := func() {
			data, err := servingcert.Generate([]string{"kubevela-cluster-gateway-service", "kubevela-cluster-gateway-service.vela-system.svc"},
				time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
			Expect(err).NotTo(HaveOccurred())
			delete(data, servingcert.CAKeyKey)
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: testGatewayNamespace, Name: testGatewaySecretName}}
			if err := c.Get(ctx, ctrlclient.ObjectKeyFromObject(secret), secret); err == nil {
				secret.Data = data
				Expect(c.Update(ctx, secret)).To(Succeed())
			} else {
				secret.Data = data
				Expect(c.Create(ctx, secret)).To(Succeed())
			}
		}

		It("does not restart while the CA stays the same", func() {
			caFile := filepath.Join(GinkgoT().TempDir(), "ca")
			trusted, err := setupClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecretName, caFile)
			Expect(err).NotTo(HaveOccurred())

			resync := clusterGatewayTLSResync(c, testGatewayNamespace, testGatewaySecretName, caFile, trusted, restart)
			resync(ctx)
			resync(ctx)
			Expect(atomic.LoadInt32(&restarts)).To(BeZero())
		})

		It("restarts in direct mode when the CA is replaced", func() {
			caFile := filepath.Join(GinkgoT().TempDir(), "ca")
			trusted, err := setupClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecretName, caFile)
			Expect(err).NotTo(HaveOccurred())
			putExpiringJobSecret()

			clusterGatewayTLSResync(c, testGatewayNamespace, testGatewaySecretName, caFile, trusted, restart)(ctx)

			Expect(atomic.LoadInt32(&restarts)).To(Equal(int32(1)))
			written, err := os.ReadFile(caFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(servingcert.Covers(written, secretCA())).To(BeTrue(), "the CA file trusts the new CA")
		})

		It("does not restart for the aggregator path", func() {
			trusted, err := setupClusterGatewayTLS(ctx, c, testGatewayNamespace, testGatewaySecretName, "")
			Expect(err).NotTo(HaveOccurred())
			putExpiringJobSecret()

			clusterGatewayTLSResync(c, testGatewayNamespace, testGatewaySecretName, "", trusted, restart)(ctx)
			Expect(atomic.LoadInt32(&restarts)).To(BeZero())
		})
	})
})

var _ = Describe("parseSecretRef", func() {
	It("splits <namespace>/<name>", func() {
		namespace, name, err := parseSecretRef("some-flag", "vela-system/my-secret")
		Expect(err).NotTo(HaveOccurred())
		Expect(namespace).To(Equal("vela-system"))
		Expect(name).To(Equal("my-secret"))
	})

	DescribeTable("rejects malformed references", func(ref string) {
		_, _, err := parseSecretRef("some-flag", ref)
		Expect(err).To(MatchError(ContainSubstring("--some-flag")))
	},
		Entry("no namespace", "my-secret"),
		Entry("empty namespace", "/my-secret"),
		Entry("empty name", "vela-system/"),
		Entry("empty", ""),
	)
})

var _ = Describe("retryStartup", func() {
	BeforeEach(func() {
		orig := startupRetryInterval
		startupRetryInterval = time.Millisecond
		DeferCleanup(func() { startupRetryInterval = orig })
	})

	It("retries until fn succeeds", func() {
		calls := 0
		Expect(retryStartup(context.Background(), "test", func() error {
			calls++
			if calls < 3 {
				return errors.New("APIService not found yet")
			}
			return nil
		})).To(Succeed())
		Expect(calls).To(Equal(3))
	})

	It("returns the last error after the timeout", func() {
		orig := startupRetryTimeout
		startupRetryTimeout = 20 * time.Millisecond
		DeferCleanup(func() { startupRetryTimeout = orig })

		Expect(retryStartup(context.Background(), "test", func() error { return errors.New("never") })).
			To(MatchError("never"))
	})
})

var _ = Describe("restartForNewCA", func() {
	It("stops once, after the jitter, however often it is called", func() {
		orig := restartJitter
		restartJitter = 10 * time.Millisecond
		DeferCleanup(func() { restartJitter = orig })

		var stops int32
		restart := restartForNewCA(func() { atomic.AddInt32(&stops, 1) })
		restart()
		restart()

		Eventually(func() int32 { return atomic.LoadInt32(&stops) }).Should(Equal(int32(1)))
		Consistently(func() int32 { return atomic.LoadInt32(&stops) }, 50*time.Millisecond).Should(Equal(int32(1)))
	})
})
