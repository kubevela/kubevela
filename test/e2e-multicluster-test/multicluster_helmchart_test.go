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

package e2e_multicluster_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kubevela/pkg/multicluster"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// helmReleases lists the Helm releases in a namespace the way `helm list -n <ns>` would on
// the given cluster. Managed clusters are read through cluster-gateway, as workerCtx is; each
// cluster gets its own discovery cache, since every proxied cluster shares the hub's host.
func helmReleases(g Gomega, cluster, namespace string) map[string]*release.Release {
	flags := genericclioptions.NewConfigFlags(false)
	cacheDir := filepath.Join(os.TempDir(), "helmchart-mc-discovery", cluster)
	flags.CacheDir = &cacheDir
	flags.Namespace = &namespace
	if cluster != multicluster.Local {
		flags.WrapConfigFn = func(c *rest.Config) *rest.Config {
			c.Wrap(multicluster.NewTransportWrapper(multicluster.ForCluster(cluster)))
			return c
		}
	}
	cfg := &action.Configuration{}
	g.Expect(cfg.Init(flags, namespace, "secret", func(string, ...interface{}) {})).Should(Succeed())
	list := action.NewList(cfg)
	list.All = true
	rels, err := list.Run()
	g.Expect(err).Should(Succeed())
	byName := map[string]*release.Release{}
	for _, r := range rels {
		byName[r.Name] = r
	}
	return byName
}

// workflowMessages reports each workflow step's phase and message, for a failure to read the cause from.
func workflowMessages(app *v1beta1.Application) string {
	if app.Status.Workflow == nil {
		return "no workflow status"
	}
	msg := app.Status.Workflow.Message
	for _, step := range app.Status.Workflow.Steps {
		msg += fmt.Sprintf("; %s %s: %s", step.Name, step.Phase, step.Message)
	}
	return msg
}

func podinfoComponent(name, releaseName, namespace string, replicas int) common.ApplicationComponent {
	raw, err := json.Marshal(map[string]interface{}{
		"chart": map[string]interface{}{
			"source":  "podinfo",
			"repoURL": "https://stefanprodan.github.io/podinfo",
			"version": "6.11.1",
		},
		"release": map[string]interface{}{"name": releaseName, "namespace": namespace},
		"values":  map[string]interface{}{"replicaCount": replicas},
	})
	Expect(err).Should(Succeed())
	return common.ApplicationComponent{Name: name, Type: "helmchart", Properties: &runtime.RawExtension{Raw: raw}}
}

func topologyPolicy(clusters ...string) v1beta1.AppPolicy {
	raw, err := json.Marshal(v1alpha1.TopologyPolicySpec{Placement: v1alpha1.Placement{Clusters: clusters}})
	Expect(err).Should(Succeed())
	return v1beta1.AppPolicy{Name: "topology", Type: v1alpha1.TopologyPolicyType, Properties: &runtime.RawExtension{Raw: raw}}
}

var _ = Describe("Test helmchart component on managed clusters", func() {

	const releaseName = "podinfo"
	const helmTimeout = 3 * time.Minute

	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	newApp := func(replicas int, clusters ...string) *v1beta1.Application {
		return &v1beta1.Application{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ApplicationKind},
			ObjectMeta: metav1.ObjectMeta{Name: "helmchart-mc", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{podinfoComponent("podinfo", releaseName, namespace, replicas)},
				Policies:   []v1beta1.AppPolicy{topologyPolicy(clusters...)},
			},
		}
	}

	waitRunning := func(key client.ObjectKey, generation int64) {
		Eventually(func(g Gomega) {
			app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
			g.Expect(app.Status.ObservedGeneration).Should(BeNumerically(">=", generation))
			g.Expect(app.Status.Phase).Should(Equal(common.ApplicationRunning), "workflow: %s", workflowMessages(app))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	}

	deployment := func(ctx context.Context) (*appsv1.Deployment, error) {
		d := &appsv1.Deployment{}
		return d, k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: releaseName}, d)
	}

	It("installs, upgrades and uninstalls the release on the cluster the topology targets", func() {
		app := newApp(1, WorkerClusterName)
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		waitRunning(key, 1)

		By("the release is recorded on the worker, as helm list there shows it")
		Eventually(func(g Gomega) {
			rel, ok := helmReleases(g, WorkerClusterName, namespace)[releaseName]
			g.Expect(ok).Should(BeTrue())
			g.Expect(rel.Info.Status).Should(Equal(release.StatusDeployed))
			g.Expect(rel.Version).Should(Equal(1))
			g.Expect(rel.Chart.Metadata.Version).Should(Equal("6.11.1"))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
		d, err := deployment(workerCtx)
		Expect(err).Should(Succeed())
		Expect(*d.Spec.Replicas).Should(BeEquivalentTo(1))

		By("the hub holds neither the release nor its resources")
		Expect(helmReleases(Default, multicluster.Local, namespace)).Should(BeEmpty())
		_, err = deployment(hubCtx)
		Expect(kerrors.IsNotFound(err)).Should(BeTrue())

		By("vela status --tree places the chart's resources on the worker")
		out, err := execCommand("status", app.Name, "-n", namespace, "--tree")
		Expect(err).Should(Succeed())
		Expect(out).Should(ContainSubstring(WorkerClusterName))
		Expect(out).Should(ContainSubstring("Deployment/" + releaseName))

		By("vela live-diff renders both sides without installing anything")
		bs, err := yaml.Marshal(newApp(1, WorkerClusterName))
		Expect(err).Should(Succeed())
		file := filepath.Join(GinkgoT().TempDir(), "app.yaml")
		Expect(os.WriteFile(file, bs, 0600)).Should(Succeed())
		out, err = execCommand("live-diff", "-f", file, "-n", namespace)
		Expect(err).Should(Succeed())
		Expect(out).ShouldNot(ContainSubstring("helm.sh/release.v1"))
		Expect(helmReleases(Default, multicluster.Local, namespace)).Should(BeEmpty())
		rel, ok := helmReleases(Default, WorkerClusterName, namespace)[releaseName]
		Expect(ok).Should(BeTrue())
		Expect(rel.Version).Should(Equal(1))

		By("changing the values upgrades the worker's release in place")
		Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
		app.Spec.Components[0] = podinfoComponent("podinfo", releaseName, namespace, 2)
		Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
		waitRunning(key, app.Generation)
		Eventually(func(g Gomega) {
			rel, ok := helmReleases(g, WorkerClusterName, namespace)[releaseName]
			g.Expect(ok).Should(BeTrue())
			g.Expect(rel.Version).Should(Equal(2))
			d, err := deployment(workerCtx)
			g.Expect(err).Should(Succeed())
			g.Expect(*d.Spec.Replicas).Should(BeEquivalentTo(2))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
		Expect(helmReleases(Default, multicluster.Local, namespace)).Should(BeEmpty())

		By("deleting the Application uninstalls the release from the worker")
		Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, key, &v1beta1.Application{}))).Should(BeTrue())
			g.Expect(helmReleases(g, WorkerClusterName, namespace)).Should(BeEmpty())
			_, err := deployment(workerCtx)
			g.Expect(kerrors.IsNotFound(err)).Should(BeTrue())
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	})

	It("keeps a separate release on each cluster sharing a release name", func() {
		app := newApp(1, multicluster.Local, WorkerClusterName)
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		waitRunning(key, 1)

		expectRevision := func(revision int) {
			Eventually(func(g Gomega) {
				for _, cluster := range []string{multicluster.Local, WorkerClusterName} {
					rel, ok := helmReleases(g, cluster, namespace)[releaseName]
					g.Expect(ok).Should(BeTrue(), "no release on %s", cluster)
					g.Expect(rel.Info.Status).Should(Equal(release.StatusDeployed))
					g.Expect(rel.Version).Should(Equal(revision), "on %s", cluster)
				}
			}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
		}
		expectRevision(1)

		By("an upgrade reaches both clusters")
		Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
		app.Spec.Components[0] = podinfoComponent("podinfo", releaseName, namespace, 2)
		Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
		waitRunning(key, app.Generation)
		expectRevision(2)
		for _, ctx := range []context.Context{hubCtx, workerCtx} {
			d, err := deployment(ctx)
			Expect(err).Should(Succeed())
			Expect(*d.Spec.Replicas).Should(BeEquivalentTo(2))
		}

		Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(helmReleases(g, multicluster.Local, namespace)).Should(BeEmpty())
			g.Expect(helmReleases(g, WorkerClusterName, namespace)).Should(BeEmpty())
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	})

	It("vela dry-run renders the chart without installing it on any cluster", func() {
		bs, err := yaml.Marshal(newApp(1, WorkerClusterName))
		Expect(err).Should(Succeed())
		file := filepath.Join(GinkgoT().TempDir(), "app.yaml")
		Expect(os.WriteFile(file, bs, 0600)).Should(Succeed())

		out, err := execCommand("dry-run", "-f", file, "-n", namespace)
		Expect(err).Should(Succeed())
		Expect(out).Should(ContainSubstring("kind: Deployment"))
		Expect(out).Should(ContainSubstring("name: " + releaseName))

		Expect(helmReleases(Default, multicluster.Local, namespace)).Should(BeEmpty())
		Expect(helmReleases(Default, WorkerClusterName, namespace)).Should(BeEmpty())
		for _, ctx := range []context.Context{hubCtx, workerCtx} {
			_, err := deployment(ctx)
			Expect(kerrors.IsNotFound(err)).Should(BeTrue())
		}
	})
})
