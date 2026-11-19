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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
)

const nacosE2ETimeout = 2 * time.Minute

func nacosE2ETemplate(endpoint, namespace string) string {
	return fmt.Sprintf(`
template: {
	nacos: {
		endpoint: {
			name:      %q
			namespace: %q
		}
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: dataId: parameter.dataId
	}
	parameter: {
		dataId: string
		group:  string
		content: {
			mysqlHost: string
			mysqlPort: int
		}
	}
}
`, endpoint, namespace)
}

// nacosE2EContent is what the server at addr holds for dataId and group: "" when it answers that
// it holds nothing, the error's text when it cannot say, so a poll retries and a one-shot check fails.
func nacosE2EContent(addr, dataID, group string) string {
	query := url.Values{"dataId": {dataID}, "group": {group}}
	httpClient := http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get("http://" + addr + "/nacos/v1/cs/configs?" + query.Encode())
	if err != nil {
		return err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return string(body)
	case http.StatusNotFound:
		return ""
	}
	return fmt.Sprintf("nacos answered %d: %s", resp.StatusCode, body)
}

// A Config applied with the client alone, nothing of the CLI, against a real Nacos server. The
// job starts the server (.github/actions/start-nacos) and says where it is: NACOS_ADDR from here,
// NACOS_POD_HOST from the controller's pods. Without them the specs skip.
var _ = Describe("Nacos through the Config controller", Label("core-config", "nacos"), func() {
	ctx := context.Background()

	var namespace, addr, podHost string

	BeforeEach(func() {
		addr, podHost = os.Getenv("NACOS_ADDR"), os.Getenv("NACOS_POD_HOST")
		if addr == "" || podHost == "" {
			Skip("NACOS_ADDR and NACOS_POD_HOST are not set")
		}
		namespace = randomNamespaceName("config-nacos-e2e")
		ns := createNamespace(ctx, namespace)
		DeferCleanup(func() {
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.Config{}, client.InNamespace(namespace))
			_ = k8sClient.DeleteAllOf(ctx, &configv1alpha1.ConfigTemplate{}, client.InNamespace(namespace))
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
		})
	})

	endpointProperties := func() map[string]interface{} {
		return map[string]interface{}{"servers": []map[string]interface{}{{"ipAddr": podHost, "port": 8848}}}
	}

	nacosContent := func(dataID, group string) string { return nacosE2EContent(addr, dataID, group) }

	template := func(endpoint string) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ct-nacos", Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: nacosE2ETemplate(endpoint, namespace)},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))
	}

	consumer := func(dataID, group string, port int) *configv1alpha1.Config {
		return &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cfg-db", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "ct-nacos", Namespace: namespace},
				Properties: configE2ERawExtension(map[string]interface{}{
					"dataId": dataID, "group": group,
					"content": map[string]interface{}{"mysqlHost": "db", "mysqlPort": port},
				}),
			},
		}
	}

	nacosSynced := func(cfg *configv1alpha1.Config) func() string {
		return func() string {
			var got configv1alpha1.Config
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got); err != nil {
				return err.Error()
			}
			c := got.Status.GetCondition(configv1alpha1.ConditionNacosSynced)
			return string(c.Status) + "/" + string(c.Reason)
		}
	}

	// A later reconcile may find the content current and say AlreadyCurrent instead of Published:
	// what counts is True, and the content the specs read from the server.
	synced := HavePrefix("True/")

	It("publishes a Config applied with the client alone under its dataId and group, and an edit again", func() {
		endpoint := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "nacos-plain", Namespace: namespace},
			Spec:       configv1alpha1.ConfigSpec{Properties: configE2ERawExtension(endpointProperties())},
		}
		Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())
		template("nacos-plain")

		dataID, group := namespace+"-db", "E2E_GROUP"
		cfg := consumer(dataID, group, 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		Eventually(nacosSynced(cfg), nacosE2ETimeout, configE2EPollInterval).Should(synced)
		Eventually(func() string { return nacosContent(dataID, group) }, configE2ETimeout, configE2EPollInterval).
			Should(Equal("mysqlHost = db\nmysqlPort = 3306\n"))
		Expect(nacosContent(dataID, "DEFAULT_GROUP")).Should(BeEmpty(), "only under the group the Config names")

		By("an edit of the Config reaches Nacos")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg)).Should(Succeed())
		edited := consumer(dataID, group, 3307)
		cfg.Spec.Properties = edited.Spec.Properties
		Expect(k8sClient.Update(ctx, cfg)).Should(Succeed())
		Eventually(func() string { return nacosContent(dataID, group) }, nacosE2ETimeout, configE2EPollInterval).
			Should(Equal("mysqlHost = db\nmysqlPort = 3307\n"))
	})

	It("uses an endpoint kept in a Secret only once its owner marks it", func() {
		props := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "nacos-kept-props", Namespace: namespace},
			StringData: map[string]string{"properties": string(configE2ERawExtension(endpointProperties()).Raw)},
		}
		Expect(k8sClient.Create(ctx, props)).Should(Succeed())
		endpoint := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "nacos-kept", Namespace: namespace},
			Spec: configv1alpha1.ConfigSpec{
				PropertiesFrom: &configv1alpha1.PropertiesReference{SecretRef: configv1alpha1.SecretKeySelector{Name: props.Name}},
			},
		}
		// the Config webhook reads the properties Secret through the manager's cache, which may
		// not have seen the Secret created a moment ago
		Eventually(func() error {
			return k8sClient.Create(ctx, endpoint.DeepCopy())
		}, configE2ETimeout, configE2EPollInterval).Should(Succeed())
		template("nacos-kept")

		dataID, group := namespace+"-kept", "E2E_GROUP"
		cfg := consumer(dataID, group, 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		Eventually(nacosSynced(cfg), nacosE2ETimeout, configE2EPollInterval).Should(Equal("False/EndpointUnreadable"))
		Expect(nacosContent(dataID, group)).Should(BeEmpty())

		By("the owner marks the endpoint Config")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(endpoint), endpoint)).Should(Succeed())
		patch := client.MergeFrom(endpoint.DeepCopy())
		metav1.SetMetaDataAnnotation(&endpoint.ObjectMeta, apitypes.AnnotationConfigNacosEndpoint, "true")
		Expect(k8sClient.Patch(ctx, endpoint, patch)).Should(Succeed())
		Eventually(nacosSynced(cfg), nacosE2ETimeout, configE2EPollInterval).Should(synced)
		Eventually(func() string { return nacosContent(dataID, group) }, configE2ETimeout, configE2EPollInterval).
			Should(Equal("mysqlHost = db\nmysqlPort = 3306\n"))
	})
})
