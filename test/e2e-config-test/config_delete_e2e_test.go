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

package config_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
)

const configE2EOutputsTemplate = `
template: {
	parameter: {
		username: string
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			username: parameter.username
		}
	}
	outputs: {
		extra: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: context.name + "-extra"
			data: username: parameter.username
		}
	}
}
`

// What kubectl delete config and GitOps pruning remove: garbage collection through the Config's
// owner references, which envtest does not run. vela config delete removes the same objects.
var _ = Describe("Deleting a Config", Label("core-config"), func() {
	ctx := context.Background()

	var namespace, target string

	BeforeEach(func() {
		namespace = randomNamespaceName("config-delete-e2e")
		target = randomNamespaceName("config-delete-target-e2e")
		for _, name := range []string{namespace, target} {
			ns := createNamespace(ctx, name)
			DeferCleanup(func() {
				_ = k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(ns.Name))
				_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.Config{}, client.InNamespace(ns.Name))
				_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.ConfigTemplate{}, client.InNamespace(ns.Name))
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
			})
		}
	})

	It("removes its output Secret, its outputs and its distribution, and recalls the distributed copy", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-outputs", Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2EOutputsTemplate},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-all", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-outputs", Namespace: namespace},
				Properties:  configE2ERawExtension(map[string]interface{}{"username": "carol"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigPhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg)
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseAvailable))

		By("distributing it the way vela config distribute does")
		distribution := config.DefaultDistributionName(cfg.Name)
		Expect(config.NewConfigFactory(k8sClient).CreateOrUpdateDistribution(ctx, namespace, distribution, &config.CreateDistributionSpec{
			Configs: []*config.NamespacedName{{Name: cfg.Name, Namespace: namespace}},
			Targets: []*config.ClusterTarget{{ClusterName: apitypes.ClusterLocalName, Namespace: target}},
		})).Should(Succeed())
		Expect(config.SetDistributionOwner(ctx, k8sClient, namespace, cfg.Name, distribution)).Should(Succeed())
		copyKey := client.ObjectKey{Namespace: target, Name: cfg.Name}
		Eventually(func() error {
			return k8sClient.Get(ctx, copyKey, &corev1.Secret{})
		}, 2*time.Minute, configE2EPollInterval).Should(Succeed(), "the distribution copies the Secret to its target")

		secretKey := client.ObjectKey{Namespace: namespace, Name: cfg.Name}
		outputKey := client.ObjectKey{Namespace: namespace, Name: cfg.Name + "-extra"}
		appKey := client.ObjectKey{Namespace: namespace, Name: distribution}
		Expect(k8sClient.Get(ctx, secretKey, &corev1.Secret{})).Should(Succeed())
		Expect(k8sClient.Get(ctx, outputKey, &corev1.ConfigMap{})).Should(Succeed())
		Expect(k8sClient.Get(ctx, appKey, &v1beta1.Application{})).Should(Succeed())

		By("deleting only the Config")
		Expect(k8sClient.Delete(ctx, cfg, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(Succeed())

		gone := func(key client.ObjectKey, obj client.Object) func() bool {
			return func() bool { return apierrors.IsNotFound(k8sClient.Get(ctx, key, obj)) }
		}
		Eventually(gone(secretKey, &corev1.Secret{}), 2*time.Minute, configE2EPollInterval).Should(BeTrue(), "the output Secret")
		Eventually(gone(outputKey, &corev1.ConfigMap{}), 2*time.Minute, configE2EPollInterval).Should(BeTrue(), "the outputs object")
		Eventually(gone(appKey, &v1beta1.Application{}), 2*time.Minute, configE2EPollInterval).Should(BeTrue(), "the distribution Application")
		Eventually(gone(copyKey, &corev1.Secret{}), 2*time.Minute, configE2EPollInterval).Should(BeTrue(), "the distributed copy")
	})
})
