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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
)

// configmap-local is the only ConfigMap reader that ships. It takes a name and
// nothing else, so a component can only ever see a ConfigMap in its own
// namespace on the cluster being rendered for.
//
// These run against the definition the chart installs rather than one the test
// authors, because the property being checked is what ships.
var _ = Describe("Built-in configmap sources e2e", func() {
	ctx := context.Background()

	var namespaceName, otherNamespaceName string
	var ns, otherNS corev1.Namespace

	BeforeEach(func() {
		namespaceName = randomNamespaceName("configmap-local-e2e")
		ns = createNamespace(ctx, namespaceName)
		otherNamespaceName = randomNamespaceName("configmap-local-other")
		otherNS = createNamespace(ctx, otherNamespaceName)
	})

	AfterEach(func() {
		By("Clean up both namespaces")
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
		Expect(k8sClient.Delete(ctx, &otherNS, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())

		// The definitions are the chart's, so cache entries carry vela-system as
		// their owning namespace and the per-test label used elsewhere in this
		// suite does not select them. The context labels do: every entry records
		// the namespace it was rendered for.
		By("Clean up source cache entries written for these namespaces")
		for _, n := range []string{namespaceName, otherNamespaceName} {
			sel := client.MatchingLabels{apitypes.LabelSourceContextPrefix + "namespace": n}
			Eventually(func() error {
				if err := k8sClient.DeleteAllOf(ctx, &corev1.Secret{},
					client.InNamespace("vela-system"), sel); err != nil {
					return err
				}
				var secrets corev1.SecretList
				if err := k8sClient.List(ctx, &secrets, client.InNamespace("vela-system"), sel); err != nil {
					return err
				}
				if len(secrets.Items) > 0 {
					return fmt.Errorf("still %d cache secrets for %s in vela-system", len(secrets.Items), n)
				}
				return nil
			}, 30*time.Second, time.Second).Should(Succeed())
		}
	})

	newApp := func(name string, sources []v1beta1.ApplicationSource, comps []oamcomm.ApplicationComponent) *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespaceName},
			Spec:       v1beta1.ApplicationSpec{Sources: sources, Components: comps},
		}
	}

	minimalComp := func() []oamcomm.ApplicationComponent {
		return []oamcomm.ApplicationComponent{
			{Name: "web", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.25.0","port":80}`)}},
		}
	}

	// Both ConfigMaps are called the same thing and hold the same key, so the only
	// thing separating them is the namespace. If configmap-local were reaching
	// anywhere but its own, this reads the wrong one rather than failing to read.
	It("reads its own namespace and not an identically named ConfigMap elsewhere", func() {
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-config", Namespace: namespaceName},
			Data:       map[string]string{"tier": "tenant"},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-config", Namespace: otherNamespaceName},
			Data:       map[string]string{"tier": "platform"},
		})).Should(Succeed())

		app := newApp("configmap-local-app", []v1beta1.ApplicationSource{
			{Name: "mine", Type: "configmap-local", Properties: &runtime.RawExtension{
				Raw: []byte(`{"name":"shared-config"}`)}},
		}, []oamcomm.ApplicationComponent{
			{Name: "web", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{
  "image":"nginx:1.25.0",
  "port":80,
  "env":[
    {"name":"MINE","value":"$(\"tier\" in source.mine.data ? source.mine.data[\"tier\"] : \"unset\")"}
  ]
}`)}},
		})
		Expect(k8sClient.Create(ctx, optIn(app))).Should(Succeed())

		verifyApplicationPhase(ctx, namespaceName, app.Name, oamcomm.ApplicationRunning)
		Eventually(func() (map[string]string, error) {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "web"}, deploy); err != nil {
				return nil, err
			}
			if len(deploy.Spec.Template.Spec.Containers) == 0 {
				return nil, fmt.Errorf("deployment has no containers")
			}
			env := map[string]string{}
			for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
				env[e.Name] = e.Value
			}
			return env, nil
		}, 90*time.Second, time.Second).Should(Equal(map[string]string{
			"MINE": "tenant",
		}))
	})

	// The reach is gone from the parameter schema, so asking for it is not a
	// permission error at render time but a rejection at admission, before the
	// Application is ever stored.
	It("rejects a namespace property", func() {
		app := newApp("configmap-local-ns", []v1beta1.ApplicationSource{
			{Name: "s", Type: "configmap-local", Properties: &runtime.RawExtension{
				Raw: []byte(`{"name":"shared-config","namespace":"vela-system"}`)}},
		}, minimalComp())
		err := k8sClient.Create(ctx, optIn(app))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not declared in the parameter schema of SourceDefinition"))
		Expect(err.Error()).To(ContainSubstring("namespace"))
	})

	It("rejects a cluster property", func() {
		app := newApp("configmap-local-cluster", []v1beta1.ApplicationSource{
			{Name: "s", Type: "configmap-local", Properties: &runtime.RawExtension{
				Raw: []byte(`{"name":"shared-config","cluster":"remote"}`)}},
		}, minimalComp())
		err := k8sClient.Create(ctx, optIn(app))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not declared in the parameter schema of SourceDefinition"))
		Expect(err.Error()).To(ContainSubstring("cluster"))
	})
})
