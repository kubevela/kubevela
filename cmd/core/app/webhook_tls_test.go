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
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/cmd/core/app/options"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/servingcert"
	webhookcerts "github.com/oam-dev/kubevela/pkg/webhook/certs"
)

const testWebhookConfigName = "kubevela-vela-core-admission"

var _ = Describe("webhookTLSOptions", func() {
	valid := func() *options.CoreOptions {
		return &options.CoreOptions{
			WebhookTLSSecret:         "vela-system/" + testWebhookConfigName,
			WebhookConfigurationName: testWebhookConfigName,
			WebhookFailurePolicy:     "Fail",
			WebhookConversionCRDs:    []string{"applications.core.oam.dev"},
		}
	}

	It("turns the flags into Ensure options", func() {
		opts, err := webhookTLSOptions(valid())
		Expect(err).NotTo(HaveOccurred())
		Expect(opts).To(Equal(webhookcerts.Options{
			SecretNamespace:   "vela-system",
			SecretName:        testWebhookConfigName,
			ConfigurationName: testWebhookConfigName,
			FailurePolicy:     admissionregistrationv1.Fail,
			ConversionCRDs:    []string{"applications.core.oam.dev"},
		}))
	})

	DescribeTable("rejects bad flags", func(mutate func(*options.CoreOptions)) {
		s := valid()
		mutate(s)
		_, err := webhookTLSOptions(s)
		Expect(err).To(HaveOccurred())
	},
		Entry("secret without a namespace", func(s *options.CoreOptions) { s.WebhookTLSSecret = testWebhookConfigName }),
		Entry("no webhook configuration", func(s *options.CoreOptions) { s.WebhookConfigurationName = "" }),
		Entry("unknown failure policy", func(s *options.CoreOptions) { s.WebhookFailurePolicy = "Sometimes" }),
	)
})

var _ = Describe("webhook serving cert", func() {
	var (
		ctx     context.Context
		c       ctrlclient.Client
		opts    webhookcerts.Options
		certDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(&admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: testWebhookConfigName},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{{
				Name:          "validating.core.oam.dev.v1beta1.applications",
				FailurePolicy: testPtr(admissionregistrationv1.Ignore),
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{Namespace: "vela-system", Name: "vela-core-webhook"},
				},
			}},
		}).Build()
		opts = webhookcerts.Options{
			SecretNamespace:   "vela-system",
			SecretName:        testWebhookConfigName,
			ConfigurationName: testWebhookConfigName,
			FailurePolicy:     admissionregistrationv1.Fail,
		}
		certDir = filepath.Join(GinkgoT().TempDir(), "serving-certs")
	})

	getWebhook := func() admissionregistrationv1.ValidatingWebhook {
		v := &admissionregistrationv1.ValidatingWebhookConfiguration{}
		Expect(c.Get(ctx, ctrlclient.ObjectKey{Name: testWebhookConfigName}, v)).To(Succeed())
		return v.Webhooks[0]
	}

	It("writes the serving cert to the cert dir, signed by the CA the webhooks trust", func() {
		Expect(ensureWebhookTLS(ctx, c, opts, certDir, false)).To(Succeed())

		crt, err := os.ReadFile(filepath.Join(certDir, servingcert.CertKey))
		Expect(err).NotTo(HaveOccurred())
		key, err := os.ReadFile(filepath.Join(certDir, servingcert.KeyKey))
		Expect(err).NotTo(HaveOccurred())
		Expect(servingcert.Check(map[string][]byte{
			servingcert.CAKey: getWebhook().ClientConfig.CABundle, servingcert.CertKey: crt, servingcert.KeyKey: key,
		}, []string{"vela-core-webhook.vela-system.svc"}, time.Now())).To(Succeed())
		Expect(*getWebhook().FailurePolicy).To(Equal(admissionregistrationv1.Ignore), "not until asked")
	})

	It("sets failurePolicy only once the webhook server is serving", func() {
		orig := startupRetryInterval
		startupRetryInterval = 5 * time.Millisecond
		DeferCleanup(func() { startupRetryInterval = orig })
		var serving atomic.Bool
		ctx, cancel := context.WithCancel(ctx)
		DeferCleanup(cancel)
		go resyncWebhookTLS(ctx, c, opts, certDir, serving.Load)

		Eventually(func() []byte { return getWebhook().ClientConfig.CABundle }).ShouldNot(BeEmpty())
		Consistently(func() admissionregistrationv1.FailurePolicyType { return *getWebhook().FailurePolicy }, 50*time.Millisecond).
			Should(Equal(admissionregistrationv1.Ignore))
		serving.Store(true)
		Eventually(func() admissionregistrationv1.FailurePolicyType { return *getWebhook().FailurePolicy }).
			Should(Equal(admissionregistrationv1.Fail))
	})
})
