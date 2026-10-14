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

package config

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/config/writer"
)

// fakeNacos stands in for the Nacos SDK client behind writer.NewNacosClient. It holds
// what was published, answers reads from it, and fails publishes on request. It is
// reached from the controller's goroutines, so it locks. A fake rather than the gomock
// client because gomock reports through the test's reporter from the calling goroutine,
// and the caller here is the manager.
type fakeNacos struct {
	config_client.IConfigClient
	mu        sync.Mutex
	store     map[string]string
	publishes []vo.ConfigParam
	failing   error // every publish fails with it while set
	reads     int
	built     int
	closed    int
}

func (f *fakeNacos) key(p vo.ConfigParam) string { return p.Group + "/" + p.DataId }

func (f *fakeNacos) GetConfig(p vo.ConfigParam) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return f.store[f.key(p)], nil
}

func (f *fakeNacos) PublishConfig(p vo.ConfigParam) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publishes = append(f.publishes, p)
	if f.failing != nil {
		return false, f.failing
	}
	f.store[f.key(p)] = p.Content
	return true, nil
}

func (f *fakeNacos) CloseClient() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

func (f *fakeNacos) publishCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.publishes)
}

func (f *fakeNacos) lastPublish() vo.ConfigParam {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.publishes[len(f.publishes)-1]
}

func (f *fakeNacos) seed(dataID, group, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[group+"/"+dataID] = content
}

func (f *fakeNacos) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads + len(f.publishes)
}

func (f *fakeNacos) clientsBuilt() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.built
}

func (f *fakeNacos) setFailing(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = err
}

// nacosTemplateCUE is the shape of pkg/config/testdata/mysql-db-nacos.cue with the
// endpoint name substituted: concrete endpoint, metadata from the parameter, properties.
func nacosTemplateCUE(endpoint string) string {
	return fmt.Sprintf(`
template: {
	nacos: {
		endpoint: {
			name:      %q
			namespace: "vela-system"
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
		stringData: {
			dataId: parameter.dataId
		}
	}
	parameter: {
		dataId: string
		group:  *"DEFAULT_GROUP" | string
		content: {
			mysqlHost: string
			mysqlPort: int
		}
	}
}
`, endpoint)
}

var _ = Describe("Config controller: the template's nacos block", func() {
	ctx := context.Background()
	const ns = "nacos-sync"
	var fake *fakeNacos
	var previousNewClient func(vo.NacosClientParam) (config_client.IConfigClient, error)

	BeforeEach(func() {
		err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
		Expect(client.IgnoreAlreadyExists(err)).Should(Succeed())
		fake = &fakeNacos{store: map[string]string{}}
		previousNewClient = writer.NewNacosClient
		writer.NewNacosClient = func(vo.NacosClientParam) (config_client.IConfigClient, error) {
			fake.mu.Lock()
			fake.built++
			fake.mu.Unlock()
			return fake, nil
		}
	})

	// Each spec's Configs go before the next spec swaps the fake in, so a Config still
	// retrying against Nacos cannot reach the next spec's fake.
	AfterEach(func() {
		var list configv1alpha1.ConfigList
		Expect(k8sClient.List(ctx, &list, client.InNamespace(ns))).Should(Succeed())
		for i := range list.Items {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &list.Items[i]))).Should(Succeed())
		}
		Eventually(func() int {
			Expect(k8sClient.List(ctx, &list, client.InNamespace(ns))).Should(Succeed())
			return len(list.Items)
		}, 30*time.Second, time.Second).Should(Equal(0))
		writer.NewNacosClient = previousNewClient
	})

	// legacyEndpoint is the Secret the 1.11 CLI writes for a config made from the
	// nacos-server template: the catalog labels and the properties under input-properties.
	legacyEndpoint := func(name string) *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "vela-system",
				Labels: map[string]string{
					apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig,
					apitypes.LabelConfigType:    "nacos-server",
				},
			},
			Data: map[string][]byte{
				legacyconfig.SaveInputPropertiesKey: []byte(`{"servers":[{"ipAddr":"127.0.0.1","port":8848}]}`),
			},
		}
	}

	availableTemplate := func(name, cueScript string) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: cueScript},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.Status.Phase
		}, 15*time.Second, time.Second).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))
	}

	newConfig := func(name, template string, port int) *configv1alpha1.Config {
		return &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: template, Namespace: ns},
				Properties: rawExtension(map[string]interface{}{
					"dataId":  name,
					"content": map[string]interface{}{"mysqlHost": "db", "mysqlPort": port},
				}),
			},
		}
	}

	nacosSynced := func(cfg *configv1alpha1.Config) func() string {
		return func() string {
			var got configv1alpha1.Config
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
			c := got.Status.GetCondition(configv1alpha1.ConditionNacosSynced)
			return string(c.Status) + "/" + string(c.Reason)
		}
	}

	It("publishes once, records the hash, and does not publish again for the same spec", func() {
		Expect(k8sClient.Create(ctx, legacyEndpoint("nacos-a"))).Should(Succeed())
		availableTemplate("nc-app-a", nacosTemplateCUE("nacos-a"))
		cfg := newConfig("nc-db-a", "nc-app-a", 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var got configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &got)
		// Published, or AlreadyCurrent when a reconcile re-ran on a cache that had not seen
		// the status write yet and read its own publish back; the publish count is the proof
		Eventually(nacosSynced(cfg), 15*time.Second, time.Second).Should(HavePrefix("True/"))

		Expect(fake.publishCount()).Should(Equal(1))
		published := fake.lastPublish()
		Expect(published.DataId).Should(Equal("nc-db-a"))
		Expect(published.Group).Should(Equal("DEFAULT_GROUP"))
		Expect(published.Type).Should(Equal("properties"))
		Expect(published.Content).Should(Equal("mysqlHost = db\nmysqlPort = 3306\n"))

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		Expect(got.Status.ExpandedWriter).ShouldNot(BeNil())
		Expect(got.Status.ExpandedWriter.Nacos).ShouldNot(BeNil())
		Expect(got.Status.ExpandedWriter.Nacos.DataID).Should(Equal("nc-db-a"))
		Expect(got.Status.ExpandedWriter.Nacos.Group).Should(Equal("DEFAULT_GROUP"))
		Expect(got.Status.ExpandedWriter.Nacos.ContentHash).Should(HaveLen(64))
		Expect(got.Status.ExpandedWriter.Nacos.LastSyncTime).ShouldNot(BeNil())
		hash := got.Status.ExpandedWriter.Nacos.ContentHash
		Expect(got.Status.GetCondition("Synced").Status).Should(Equal(corev1.ConditionTrue))

		By("a reconcile with the same spec publishes nothing")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		got.Annotations = map[string]string{"touch": "1"}
		Expect(k8sClient.Update(ctx, &got)).Should(Succeed())
		Consistently(fake.publishCount, 5*time.Second, time.Second).Should(Equal(1))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		Expect(got.Status.ExpandedWriter.Nacos.ContentHash).Should(Equal(hash))

		By("a properties change publishes once more with the new content and a new hash")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		got.Spec.Properties = rawExtension(map[string]interface{}{
			"dataId":  "nc-db-a",
			"content": map[string]interface{}{"mysqlHost": "db", "mysqlPort": 3307},
		})
		Expect(k8sClient.Update(ctx, &got)).Should(Succeed())
		Eventually(fake.publishCount, 15*time.Second, time.Second).Should(Equal(2))
		Expect(fake.lastPublish().Content).Should(Equal("mysqlHost = db\nmysqlPort = 3307\n"))
		Eventually(func() string {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
			return got.Status.ExpandedWriter.Nacos.ContentHash
		}, 15*time.Second, time.Second).ShouldNot(Equal(hash))
		Consistently(fake.publishCount, 3*time.Second, time.Second).Should(Equal(2))
	})

	It("reads before the first publish and records AlreadyCurrent when the CLI already published the content", func() {
		Expect(k8sClient.Create(ctx, legacyEndpoint("nacos-b"))).Should(Succeed())
		availableTemplate("nc-app-b", nacosTemplateCUE("nacos-b"))
		// what the 1.11 CLI wrote: same pairs, its own order and spacing
		fake.seed("nc-db-b", "DEFAULT_GROUP", "mysqlPort = 3306\nmysqlHost = db\n")
		cfg := newConfig("nc-db-b", "nc-app-b", 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(nacosSynced(cfg), 15*time.Second, time.Second).Should(Equal("True/AlreadyCurrent"))
		Consistently(fake.publishCount, 3*time.Second, time.Second).Should(Equal(0))

		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		Expect(got.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		Expect(got.Status.ExpandedWriter.Nacos.ContentHash).Should(HaveLen(64))
		Expect(got.Status.ExpandedWriter.Nacos.LastSyncTime).ShouldNot(BeNil())
	})

	It("reads the endpoint from the Secret a Config CR materialises", func() {
		endpoint := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "nacos-c", Namespace: "vela-system"},
			Spec: configv1alpha1.ConfigSpec{
				Properties: rawExtension(map[string]interface{}{
					"servers": []map[string]interface{}{{"ipAddr": "127.0.0.1", "port": 8848}},
				}),
			},
		}
		Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())
		var gotEndpoint configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(endpoint), configv1alpha1.ConfigPhaseAvailable, &gotEndpoint)

		availableTemplate("nc-app-c", nacosTemplateCUE("nacos-c"))
		cfg := newConfig("nc-db-c", "nc-app-c", 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(nacosSynced(cfg), 15*time.Second, time.Second).Should(HavePrefix("True/"))
		Expect(fake.publishCount()).Should(Equal(1))
	})

	It("reports EndpointNotFound, stays Available, and converges once the endpoint exists", func() {
		availableTemplate("nc-app-d", nacosTemplateCUE("nacos-d"))
		cfg := newConfig("nc-db-d", "nc-app-d", 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(nacosSynced(cfg), 15*time.Second, time.Second).Should(Equal("False/EndpointNotFound"))
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		Expect(got.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		Expect(got.Status.SecretRef).ShouldNot(BeNil())
		Expect(got.Status.ExpandedWriter).Should(BeNil())
		Expect(got.Status.GetCondition(configv1alpha1.ConditionNacosSynced).Message).Should(ContainSubstring("vela-system/nacos-d"))
		Expect(fake.calls()).Should(Equal(0))

		By("the endpoint appears and the requeue converges")
		Expect(k8sClient.Create(ctx, legacyEndpoint("nacos-d"))).Should(Succeed())
		Eventually(nacosSynced(cfg), 60*time.Second, time.Second).Should(HavePrefix("True/"))
		Expect(fake.publishCount()).Should(Equal(1))
	})

	It("refuses a sensitive endpoint config", func() {
		sensitive := legacyEndpoint("nacos-e")
		sensitive.Annotations = map[string]string{apitypes.AnnotationConfigSensitive: "true"}
		Expect(k8sClient.Create(ctx, sensitive)).Should(Succeed())
		availableTemplate("nc-app-e", nacosTemplateCUE("nacos-e"))
		cfg := newConfig("nc-db-e", "nc-app-e", 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(nacosSynced(cfg), 15*time.Second, time.Second).Should(Equal("False/EndpointUnreadable"))
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		Expect(got.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		Expect(got.Status.GetCondition(configv1alpha1.ConditionNacosSynced).Message).Should(ContainSubstring("sensitive"))
		Expect(fake.calls()).Should(Equal(0))
	})

	It("retries a failed publish with backoff and records each attempt on the condition", func() {
		Expect(k8sClient.Create(ctx, legacyEndpoint("nacos-f"))).Should(Succeed())
		availableTemplate("nc-app-f", nacosTemplateCUE("nacos-f"))
		fake.setFailing(errors.New("nacos: 503"))
		cfg := newConfig("nc-db-f", "nc-app-f", 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		Eventually(nacosSynced(cfg), 15*time.Second, time.Second).Should(Equal("False/PublishFailed"))
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		Expect(got.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		Expect(got.Status.SecretRef).ShouldNot(BeNil())
		Expect(got.Status.GetCondition(configv1alpha1.ConditionNacosSynced).Message).Should(ContainSubstring("nacos: 503"))
		Expect(got.Status.ExpandedWriter).Should(BeNil(), "no successful sync recorded yet")
		By("the requeue keeps trying while Nacos keeps failing")
		Eventually(fake.publishCount, 15*time.Second, time.Second).Should(BeNumerically(">=", 2))

		By("Nacos recovers and the next retry publishes")
		fake.setFailing(nil)
		Eventually(nacosSynced(cfg), 60*time.Second, time.Second).Should(HavePrefix("True/"))
		Expect(fake.lastPublish().Content).Should(Equal("mysqlHost = db\nmysqlPort = 3306\n"))
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &got)).Should(Succeed())
		Expect(got.Status.ExpandedWriter.Nacos.ContentHash).Should(HaveLen(64))
	})

	It("never touches the client for a template without the block, and deleting a Config calls nothing", func() {
		availableTemplate("plain-g", testCUETemplate)
		cfg := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "plain-cfg-g", Namespace: ns},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "plain-g", Namespace: ns},
				Properties:  rawExtension(map[string]string{"username": "alice"}),
			},
		}
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		var got configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseAvailable, &got)
		Expect(got.Status.ExpandedWriter).Should(BeNil())
		for _, c := range got.Status.Conditions {
			Expect(c.Type).ShouldNot(Equal(configv1alpha1.ConditionNacosSynced), "no NacosSynced condition for a template without the block")
		}
		Expect(fake.calls()).Should(Equal(0))
		Expect(fake.clientsBuilt()).Should(Equal(0))

		By("deleting a nacos-backed Config removes nothing from Nacos")
		Expect(k8sClient.Create(ctx, legacyEndpoint("nacos-g"))).Should(Succeed())
		availableTemplate("nc-app-g", nacosTemplateCUE("nacos-g"))
		nc := newConfig("nc-db-g", "nc-app-g", 3306)
		Expect(k8sClient.Create(ctx, nc)).Should(Succeed())
		Eventually(nacosSynced(nc), 15*time.Second, time.Second).Should(HavePrefix("True/"))
		calls := fake.calls()
		Expect(k8sClient.Delete(ctx, nc)).Should(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(nc), &got)
			return err != nil
		}, 15*time.Second, time.Second).Should(BeTrue())
		Consistently(fake.calls, 3*time.Second, time.Second).Should(Equal(calls))
		fake.mu.Lock()
		Expect(fake.store["DEFAULT_GROUP/nc-db-g"]).Should(Equal("mysqlHost = db\nmysqlPort = 3306\n"))
		fake.mu.Unlock()
	})

	It("puts the Config in Error when the template's nacos block fails validation", func() {
		// no endpoint: the webhook would deny this template; with the webhooks off it
		// reaches the cluster and the reconciler must say what is wrong
		availableTemplate("nc-bad-h", `
template: {
	nacos: {
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  "DEFAULT_GROUP"
		}
		content: parameter.content
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
	}
	parameter: {
		dataId: string
		content: {...}
	}
}
`)
		cfg := newConfig("nc-db-h", "nc-bad-h", 3306)
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())

		var got configv1alpha1.Config
		eventuallyConfigPhase(ctx, client.ObjectKeyFromObject(cfg), configv1alpha1.ConfigPhaseError, &got)
		Expect(got.Status.GetCondition("Synced").Message).Should(ContainSubstring("nacos.endpoint.name is required"))
		Expect(fake.calls()).Should(Equal(0))
	})
})
