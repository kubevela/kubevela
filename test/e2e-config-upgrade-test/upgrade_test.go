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
package upgrade_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// cliConfigs are the configs the 1.11 CLI writes; workflowConfig is the one a workflow step does.
// An Application owns what its step dispatched, so its state-keep may write that Secret again
// under the new controller, with the same content: only the CLI's are held to the same
// resourceVersion.
var cliConfigs = []string{"up-plain-cfg", "up-sensitive-cfg", "up-outputs-cfg", "up-dist-cfg", "up-nacos-cfg"}

const (
	workflowConfig = "up-workflow"
	nacosDataID    = "up-db"
	nacosGroup     = "UPGRADE_E2E"
	lateConfig     = "up-late-cfg"
)

var seedTemplates = []string{"up-plain", "up-sensitive", "up-outputs", "up-addon", "up-nacos-db"}

func allConfigs() []string { return append(append([]string{}, cliConfigs...), workflowConfig) }

func nacosContent() string {
	query := url.Values{"dataId": {nacosDataID}, "group": {nacosGroup}}
	httpClient := http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get("http://" + os.Getenv("NACOS_ADDR") + "/nacos/v1/cs/configs?" + query.Encode())
	if err != nil {
		return err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	return string(body)
}

// publishedProperties is the content the 1.11 CLI published, as sorted lines: it writes properties
// keys in random order, and the controller compares them as pairs, not as text.
func publishedProperties() []string {
	lines := strings.Split(strings.TrimSpace(nacosContent()), "\n")
	sort.Strings(lines)
	return lines
}

func stateOf(secret *corev1.Secret) string {
	return string(secret.UID) + " " + secret.ResourceVersion
}

// listsAll checks that a list command names each of names.
func listsAll(out string, names ...string) {
	for _, name := range names {
		ExpectWithOffset(1, out).Should(ContainSubstring(name))
	}
}

// consumerWorks re-runs the Application reading the plain config on the controller now running:
// its workflow finishes on the new publish version, and its output, removed first, is back.
func consumerWorks(publishVersion string) {
	out := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: seedNS, Name: "up-reader-out"}}
	Expect(k8sClient.Delete(ctx, out)).Should(Succeed())
	var app v1beta1.Application
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: "up-reader"}, &app)).Should(Succeed())
	patch := client.MergeFrom(app.DeepCopy())
	metav1.SetMetaDataAnnotation(&app.ObjectMeta, "app.oam.dev/publishVersion", publishVersion)
	Expect(k8sClient.Patch(ctx, &app, patch)).Should(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&app), &app)).Should(Succeed())
		g.Expect(app.Status.Workflow).ShouldNot(BeNil())
		g.Expect(app.Status.Workflow.AppRevision).Should(Equal(publishVersion))
		g.Expect(app.Status.Workflow.Finished).Should(BeTrue())
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(out), out)).Should(Succeed())
		g.Expect(out.Data).Should(HaveKeyWithValue("value", "v-plain"))
	}, waitTimeout, waitPoll).Should(Succeed())
}

var _ = Describe("Upgrading config storage from 1.11", func() {
	It("seeds each kind of legacy object the verification plan lists", Label("seed"), func() {
		for _, ns := range []string{seedNS, targetNS} {
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}))).Should(Succeed())
		}
		for _, name := range seedTemplates {
			oldCLI("config-template", "apply", "-f", testdata(name+".cue"), "-n", seedNS)
		}
		oldCLI("config-template", "apply", "-f", testdata("up-nacos-server.cue"), "-n", "vela-system")

		By("an addon-shipped template: the marks the addon's Application leaves, set by hand")
		var addon corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: "config-template-up-addon"}, &addon)).Should(Succeed())
		patch := client.MergeFrom(addon.DeepCopy())
		addon.Labels["app.oam.dev/name"], addon.Labels["app.oam.dev/namespace"] = "addon-up", "vela-system"
		Expect(k8sClient.Patch(ctx, &addon, patch)).Should(Succeed())

		oldCLI("config", "create", "up-plain-cfg", "-t", seedNS+"/up-plain", "-n", seedNS, "value=v-plain")
		oldCLI("config", "create", "up-sensitive-cfg", "-t", seedNS+"/up-sensitive", "-n", seedNS, "value=v-sensitive")
		oldCLI("config", "create", "up-outputs-cfg", "-t", seedNS+"/up-outputs", "-n", seedNS, "value=v-outputs")
		oldCLI("config", "create", "up-dist-cfg", "-t", seedNS+"/up-plain", "-n", seedNS, "value=v-dist")
		oldCLI("config", "distribute", "up-dist-cfg", "-t", targetNS, "-n", seedNS)
		oldCLI("config", "create", "up-nacos", "-t", "vela-system/up-nacos-server", "-n", "vela-system",
			"servers[0].ipAddr="+os.Getenv("NACOS_CLI_HOST"), "servers[0].port=8848")
		oldCLI("config", "create", "up-nacos-cfg", "-t", seedNS+"/up-nacos-db", "-n", seedNS,
			"dataId="+nacosDataID, "content.mysqlHost=db", "content.mysqlPort=3306")
		Eventually(publishedProperties, waitTimeout, waitPoll).Should(Equal([]string{"mysqlHost = db", "mysqlPort = 3306"}))

		By("the workflow step's config and the Application consuming the plain one")
		raw, err := os.ReadFile(testdata("applications.yaml"))
		Expect(err).ShouldNot(HaveOccurred())
		for _, doc := range strings.Split(string(raw), "\n---\n") {
			var app v1beta1.Application
			Expect(yaml.Unmarshal([]byte(doc), &app)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &app)).Should(Succeed())
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: workflowConfig}, &corev1.Secret{})
		}, waitTimeout, waitPoll).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: "up-reader-out"}, &corev1.ConfigMap{})
		}, waitTimeout, waitPoll).Should(Succeed())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: targetNS, Name: "up-dist-cfg"}, &corev1.Secret{})
		}, waitTimeout, waitPoll).Should(Succeed())

		state := map[string]string{}
		for _, name := range allConfigs() {
			state[name] = stateOf(getSecret(seedNS, name))
			Expect(getSecret(seedNS, name).OwnerReferences).Should(BeEmpty(), "legacy: nothing owns it")
		}
		saveState(state)
	})

	Context("after upgrading to the branch, gate off", Label("after-upgrade"), func() {
		It("leaves the Secrets as they were (check 2)", func() {
			state := loadState()
			Consistently(func(g Gomega) {
				for _, name := range cliConfigs {
					g.Expect(stateOf(getSecret(seedNS, name))).Should(Equal(state[name]), "same UID and resourceVersion: %s", name)
				}
			}, "30s", waitPoll).Should(Succeed())
			Expect(string(getSecret(seedNS, workflowConfig).UID)).Should(Equal(strings.Fields(state[workflowConfig])[0]))
			Expect(getSecret(seedNS, workflowConfig).Data).Should(HaveKeyWithValue("value", []byte("v-workflow")))
			var configs configv1alpha1.ConfigList
			Expect(k8sClient.List(ctx, &configs, client.InNamespace(seedNS))).Should(Succeed())
			Expect(configs.Items).Should(BeEmpty(), "nothing is migrated with the gate off")
		})

		It("keeps the consuming Application working (check 2)", func() {
			consumerWorks("after-upgrade")
		})

		It("lists, shows and deletes with both CLIs (check 1)", func() {
			for _, cli := range []func(...string) string{oldCLI, newCLI} {
				listsAll(cli("config", "list", "-n", seedNS), allConfigs()...)
				listsAll(cli("config-template", "list", "-n", seedNS), seedTemplates...)
				Expect(cli("config-template", "show", "up-plain", "-n", seedNS)).Should(ContainSubstring("value"))
			}
			for i, cli := range []func(...string) string{oldCLI, newCLI} {
				name := fmt.Sprintf("up-delete-%d", i)
				oldCLI("config", "create", name, "-t", seedNS+"/up-plain", "-n", seedNS, "value=v")
				cli("config", "delete", name, "-n", seedNS, "-y")
				Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: name}, &corev1.Secret{}))).Should(BeTrue())

				template := fmt.Sprintf("up-delete-template-%d", i)
				oldCLI("config-template", "apply", "-f", testdata("up-plain.cue"), "--name", template, "-n", seedNS)
				cli("config-template", "delete", template, "-n", seedNS, "-y")
				Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: "config-template-" + template}, &corev1.ConfigMap{}))).Should(BeTrue())
			}
		})
	})

	It("adopts each object once with migrate --all, and a second run changes nothing (check 3)", Label("migrate"), func() {
		state := loadState()
		first := newCLI("config-template", "migrate", "--all", "-n", "vela-system")
		Expect(first).Should(ContainSubstring("adopted 2, skipped 0, conflict 0, failed 0"), first)
		first = newCLI("config-template", "migrate", "--all", "-n", seedNS)
		Expect(first).Should(ContainSubstring("adopted 11, skipped 0, conflict 0, failed 0"), first)

		uids := map[string]string{}
		for _, name := range allConfigs() {
			var cfg configv1alpha1.Config
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: name}, &cfg)).Should(Succeed())
				g.Expect(cfg.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable), name)
			}, waitTimeout, waitPoll).Should(Succeed())
			uids[name] = string(cfg.UID)
			secret := getSecret(seedNS, name)
			Expect(string(secret.UID)).Should(Equal(strings.Fields(state[name])[0]), "the same Secret: %s", name)
			Expect(metav1.IsControlledBy(secret, &cfg)).Should(BeTrue(), name)
		}
		Expect(getSecret(seedNS, "up-sensitive-cfg").Annotations).Should(HaveKeyWithValue("config.oam.dev/sensitive", "true"))
		Expect(getSecret(seedNS, "up-plain-cfg").Data).Should(HaveKeyWithValue("value", []byte("v-plain")))
		var dist v1beta1.Application
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: "distribute-up-dist-cfg"}, &dist)).Should(Succeed())
		Expect(metav1.GetControllerOf(&dist)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(&dist).Name).Should(Equal("up-dist-cfg"))

		second := newCLI("config-template", "migrate", "--all", "-n", seedNS)
		Expect(second).Should(ContainSubstring("adopted 0, skipped 11, conflict 0, failed 0"), second)
		second = newCLI("config-template", "migrate", "--all", "-n", "vela-system")
		Expect(second).Should(ContainSubstring("adopted 0, skipped 2, conflict 0, failed 0"), second)
		for name, uid := range uids {
			var cfg configv1alpha1.Config
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: name}, &cfg)).Should(Succeed())
			Expect(string(cfg.UID)).Should(Equal(uid), name)
		}
		consumerWorks("after-migrate")
	})

	It("adopts what is left once with the gate on, and a further restart changes nothing (check 4)", Label("gate"), func() {
		oldCLI("config", "create", lateConfig, "-t", seedNS+"/up-plain", "-n", seedNS, "value=v-late")
		chart, err := filepath.Abs("../../charts/vela-core")
		Expect(err).ShouldNot(HaveOccurred())
		mustRun("helm", "upgrade", "kubevela", chart, "-n", "vela-system", "--reuse-values",
			"--set", "featureGates.autoMigrateLegacyConfigs=true", "--wait", "--timeout", "5m")
		mustRun("kubectl", "-n", "vela-system", "rollout", "status", "deploy/kubevela-vela-core", "--timeout=180s")

		var late configv1alpha1.Config
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: seedNS, Name: lateConfig}, &late)).Should(Succeed())
			g.Expect(late.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		}, waitTimeout, waitPoll).Should(Succeed())
		versions := func() map[string]string {
			got := map[string]string{}
			for _, name := range append([]string{lateConfig}, cliConfigs...) {
				got[name] = getSecret(seedNS, name).ResourceVersion
			}
			var copied corev1.Secret
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: targetNS, Name: "up-dist-cfg"}, &copied)).Should(Succeed())
			got["distributed copy"] = copied.ResourceVersion
			return got
		}
		before := versions()

		mustRun("kubectl", "-n", "vela-system", "rollout", "restart", "deploy/kubevela-vela-core")
		mustRun("kubectl", "-n", "vela-system", "rollout", "status", "deploy/kubevela-vela-core", "--timeout=180s")
		// the sweep runs on the leader, which may take the lease well after the rollout
		Eventually(func() string {
			out, _ := run("kubectl", "-n", "vela-system", "logs", "deploy/kubevela-vela-core")
			return out
		}, waitTimeout, waitPoll).Should(ContainSubstring("legacy config migration sweep finished"))
		Consistently(func(g Gomega) {
			g.Expect(versions()).Should(Equal(before))
			var again configv1alpha1.Config
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&late), &again)).Should(Succeed())
			g.Expect(again.UID).Should(Equal(late.UID))
		}, "45s", waitPoll).Should(Succeed())
	})

	It("keeps Nacos content, and an edit through the Config reaches it (check 5)", Label("nacos"), func() {
		Expect(publishedProperties()).Should(Equal([]string{"mysqlHost = db", "mysqlPort = 3306"}))
		var cfg configv1alpha1.Config
		key := client.ObjectKey{Namespace: seedNS, Name: "up-nacos-cfg"}
		Expect(k8sClient.Get(ctx, key, &cfg)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, key, &cfg)).Should(Succeed())
			g.Expect(cfg.Status.GetCondition(configv1alpha1.ConditionNacosSynced).Reason).Should(BeEquivalentTo(configv1alpha1.ReasonNacosAlreadyCurrent), "nothing published again")
		}, waitTimeout, waitPoll).Should(Succeed())

		patch := client.MergeFrom(cfg.DeepCopy())
		cfg.Spec.Properties.Raw = []byte(fmt.Sprintf(`{"dataId":%q,"content":{"mysqlHost":"db","mysqlPort":3307}}`, nacosDataID))
		Expect(k8sClient.Patch(ctx, &cfg, patch)).Should(Succeed())
		Eventually(nacosContent, waitTimeout, waitPoll).Should(Equal("mysqlHost = db\nmysqlPort = 3307\n"))
	})

	Context("after rolling back to 1.11.0", Label("after-rollback"), func() {
		It("keeps the legacy objects, served by the old CLI (check 6)", func() {
			state := loadState()
			for _, name := range allConfigs() {
				Expect(string(getSecret(seedNS, name).UID)).Should(Equal(strings.Fields(state[name])[0]), name)
			}
			Expect(getSecret(seedNS, "up-sensitive-cfg").Annotations).Should(HaveKeyWithValue("config.oam.dev/sensitive", "true"))
			listsAll(oldCLI("config", "list", "-n", seedNS), append(allConfigs(), lateConfig)...)
			listsAll(oldCLI("config-template", "list", "-n", seedNS), seedTemplates...)
			Expect(oldCLI("config-template", "show", "up-sensitive", "-n", seedNS)).Should(ContainSubstring("value"))
			oldCLI("config", "create", "up-after-rollback", "-t", seedNS+"/up-plain", "-n", seedNS, "value=v-after")
			Expect(getSecret(seedNS, "up-after-rollback").Data).Should(HaveKeyWithValue("value", []byte("v-after")))
		})

		It("keeps the consuming Application working (check 6)", func() {
			consumerWorks("after-rollback")
		})

		It("leaves the Config CRs in place and collects nothing", func() {
			var configs configv1alpha1.ConfigList
			Expect(k8sClient.List(ctx, &configs, client.InNamespace(seedNS))).Should(Succeed())
			Expect(len(configs.Items)).Should(Equal(len(allConfigs()) + 1))
		})
	})
})
