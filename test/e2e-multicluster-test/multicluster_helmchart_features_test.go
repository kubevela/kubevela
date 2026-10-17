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
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// The widgets chart is served from the hub, both as a plain tarball and from a registry that
// requires credentials, so these specs need nothing outside the two clusters.
const (
	chartFixturesNS = "helmchart-mc-fixtures"
	widgetsURL      = "http://charts." + chartFixturesNS + ".svc.cluster.local/widgets-0.1.0.tgz"
	widgetsOCI      = "oci://registry." + chartFixturesNS + ".svc.cluster.local:5000/charts/widgets"
	registryUser    = "test-user"
	registryPass    = "test-pass"
	widgetsCRD      = "widgets.e2e.kubevela.io"
)

// deployChartFixtures packages testdata/chart/widgets, serves it over HTTP from nginx, and
// pushes it to a registry:2 that takes basic auth (the htpasswd the core suites use).
func deployChartFixtures(ctx context.Context) {
	dir := GinkgoT().TempDir()
	out, err := exec.CommandContext(ctx, "helm", "package", "./testdata/chart/widgets", "-d", dir).CombinedOutput()
	Expect(err).Should(Succeed(), string(out))
	tgz, err := os.ReadFile(filepath.Join(dir, "widgets-0.1.0.tgz"))
	Expect(err).Should(Succeed())
	htpasswd, err := os.ReadFile("../e2e-framework/testdata/auth/htpasswd")
	Expect(err).Should(Succeed())

	labels := func(app string) map[string]string { return map[string]string{"app": app} }
	deployment := func(name, image string, port int32, env []corev1.EnvVar, mounts []corev1.VolumeMount, volumes []corev1.Volume) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: chartFixturesNS},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: labels(name)},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels(name)},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: name, Image: image, Env: env, VolumeMounts: mounts,
							Ports:          []corev1.ContainerPort{{ContainerPort: port}},
							ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(port)}}},
						}},
						Volumes: volumes,
					},
				},
			},
		}
	}
	service := func(name string, port int32) *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: chartFixturesNS},
			Spec:       corev1.ServiceSpec{Selector: labels(name), Ports: []corev1.ServicePort{{Port: port, TargetPort: intstr.FromInt32(port)}}},
		}
	}
	chartVolume := corev1.Volume{Name: "chart", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{Name: "widgets-chart"}}}}
	objects := []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: chartFixturesNS}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "widgets-chart", Namespace: chartFixturesNS},
			BinaryData: map[string][]byte{"widgets-0.1.0.tgz": tgz}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "registry-htpasswd", Namespace: chartFixturesNS},
			Data: map[string][]byte{"htpasswd": htpasswd}},
		deployment("charts", "nginx:1.27-alpine", 80, nil,
			[]corev1.VolumeMount{{Name: "chart", MountPath: "/usr/share/nginx/html"}}, []corev1.Volume{chartVolume}),
		service("charts", 80),
		deployment("registry", "registry:2", 5000,
			[]corev1.EnvVar{
				{Name: "REGISTRY_AUTH", Value: "htpasswd"},
				{Name: "REGISTRY_AUTH_HTPASSWD_REALM", Value: "e2e"},
				{Name: "REGISTRY_AUTH_HTPASSWD_PATH", Value: "/auth/htpasswd"},
			},
			[]corev1.VolumeMount{{Name: "auth", MountPath: "/auth"}},
			[]corev1.Volume{{Name: "auth", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "registry-htpasswd"}}}}),
		service("registry", 5000),
	}
	for _, obj := range objects {
		Expect(k8sClient.Create(ctx, obj)).Should(Succeed())
	}
	DeferCleanup(func(ctx SpecContext) {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: chartFixturesNS}}))).Should(Succeed())
	})
	for _, name := range []string{"charts", "registry"} {
		Eventually(func(g Gomega) {
			d := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: chartFixturesNS, Name: name}, d)).Should(Succeed())
			g.Expect(d.Status.ReadyReplicas).Should(BeEquivalentTo(1))
		}).WithPolling(2 * time.Second).WithTimeout(3 * time.Minute).Should(Succeed())
	}

	host := "registry." + chartFixturesNS + ".svc.cluster.local:5000"
	push := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "push-widgets", Namespace: chartFixturesNS},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To[int32](3),
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{{
					Name: "push", Image: "alpine/helm:3.18.4", Command: []string{"sh", "-ec"},
					Args: []string{"helm registry login " + host + " -u " + registryUser + " -p " + registryPass + " --plain-http\n" +
						"helm push /charts/widgets-0.1.0.tgz oci://" + host + "/charts --plain-http"},
					VolumeMounts: []corev1.VolumeMount{{Name: "chart", MountPath: "/charts"}},
				}},
				Volumes: []corev1.Volume{chartVolume},
			}},
		},
	}
	Expect(k8sClient.Create(ctx, push)).Should(Succeed())
	Eventually(func(g Gomega) {
		job := &batchv1.Job{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(push), job)).Should(Succeed())
		for _, c := range job.Status.Conditions {
			if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
				StopTrying("pushing the widgets chart failed: " + c.Reason + ": " + c.Message).Now()
			}
		}
		g.Expect(job.Status.Conditions).Should(ContainElement(SatisfyAll(
			HaveField("Type", batchv1.JobComplete), HaveField("Status", corev1.ConditionTrue))))
	}).WithPolling(2 * time.Second).WithTimeout(3 * time.Minute).Should(Succeed())
}

func widgetsComponent(source, version, releaseName, namespace string, size int, authSecret string) common.ApplicationComponent {
	chart := map[string]interface{}{"source": source}
	if version != "" {
		chart["version"] = version
	}
	if authSecret != "" {
		chart["auth"] = map[string]interface{}{"secretRef": map[string]interface{}{"name": authSecret}}
	}
	raw, err := json.Marshal(map[string]interface{}{
		"chart":   chart,
		"release": map[string]interface{}{"name": releaseName, "namespace": namespace},
		"values":  map[string]interface{}{"size": size},
	})
	Expect(err).Should(Succeed())
	return common.ApplicationComponent{Name: releaseName, Type: "helmchart", Properties: &runtime.RawExtension{Raw: raw}}
}

// helmReleasesAt lists releases the way `helm list -n <ns> --kubeconfig <path>` would.
func helmReleasesAt(g Gomega, kubeconfig, namespace string) map[string]*release.Release {
	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &kubeconfig
	flags.Namespace = &namespace
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

// directWorkerKubeconfig returns a kubeconfig that reaches the worker's API server without the
// hub, or "" when there is none: E2E_WORKER_DIRECT_KUBECONFIG, else the join kubeconfig when its
// server is reachable from here (on CI it is; on a laptop its in-network address may not be).
func directWorkerKubeconfig() string {
	path := os.Getenv("E2E_WORKER_DIRECT_KUBECONFIG")
	if path == "" {
		path = WorkerClusterKubeConfigPath
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return ""
	}
	cfg.Timeout = 5 * time.Second
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return ""
	}
	if _, err := cs.Discovery().ServerVersion(); err != nil {
		return ""
	}
	return path
}

var _ = Describe("Test helmchart features on managed clusters", Ordered, ContinueOnFailure, func() {
	const helmTimeout = 3 * time.Minute

	var namespace string
	var hubCtx, workerCtx context.Context

	BeforeAll(func(ctx SpecContext) {
		deployChartFixtures(ctx)
	})

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	onWorker := func(comps ...common.ApplicationComponent) *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "helmchart-features", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: comps,
				Policies:   []v1beta1.AppPolicy{topologyPolicy(WorkerClusterName)},
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

	widget := func(ctx context.Context, name string) (*unstructured.Unstructured, error) {
		w := &unstructured.Unstructured{}
		w.SetAPIVersion("e2e.kubevela.io/v1")
		w.SetKind("Widget")
		return w, k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, w)
	}

	hookRevision := func(ctx context.Context, name string) (string, error) {
		cm := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name + "-hook"}, cm); err != nil {
			return "", err
		}
		return cm.Data["revision"], nil
	}

	expectWorkerRelease := func(name string, revision, size int) {
		Eventually(func(g Gomega) {
			rel, ok := helmReleases(g, WorkerClusterName, namespace)[name]
			g.Expect(ok).Should(BeTrue())
			g.Expect(rel.Info.Status).Should(Equal(release.StatusDeployed))
			g.Expect(rel.Version).Should(Equal(revision))
			w, err := widget(workerCtx, name)
			g.Expect(err).Should(Succeed())
			g.Expect(w.Object["spec"]).Should(HaveKeyWithValue("size", BeEquivalentTo(size)))
			rev, err := hookRevision(workerCtx, name)
			g.Expect(err).Should(Succeed())
			g.Expect(rev).Should(Equal(fmt.Sprint(revision)))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	}

	It("installs a chart's CRDs, custom resources and hooks on the worker, and upgrades them there", func() {
		app := onWorker(widgetsComponent(widgetsURL, "", "widgets", namespace, 1, ""))
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		waitRunning(key, 1)

		By("the CRD from crds/, the Widget that needs it, and the post-install hook are on the worker")
		Expect(k8sClient.Get(workerCtx, types.NamespacedName{Name: widgetsCRD}, &apiextensionsv1.CustomResourceDefinition{})).Should(Succeed())
		expectWorkerRelease("widgets", 1, 1)

		By("none of them are on the hub")
		Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, types.NamespacedName{Name: widgetsCRD}, &apiextensionsv1.CustomResourceDefinition{}))).Should(BeTrue())
		Expect(helmReleases(Default, "local", namespace)).Should(BeEmpty())
		_, err := hookRevision(hubCtx, "widgets")
		Expect(kerrors.IsNotFound(err)).Should(BeTrue())

		By("an upgrade updates the Widget and reruns the hook on the worker")
		Expect(k8sClient.Get(hubCtx, key, app)).Should(Succeed())
		app.Spec.Components[0] = widgetsComponent(widgetsURL, "", "widgets", namespace, 2, "")
		Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
		waitRunning(key, app.Generation)
		expectWorkerRelease("widgets", 2, 2)

		By("deleting the Application removes the release and the Widget from the worker")
		Expect(k8sClient.Delete(hubCtx, app)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(helmReleases(g, WorkerClusterName, namespace)).Should(BeEmpty())
			_, err := widget(workerCtx, "widgets")
			g.Expect(kerrors.IsNotFound(err)).Should(BeTrue())
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	})

	It("pulls an OCI chart with credentials read on the hub, and installs it on the worker", func() {
		Expect(k8sClient.Create(hubCtx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-creds", Namespace: namespace},
			StringData: map[string]string{"username": registryUser, "password": registryPass, "insecurePlainHTTP": "true"},
		})).Should(Succeed())
		app := onWorker(widgetsComponent(widgetsOCI, "0.1.0", "widgets-oci", namespace, 3, "registry-creds"))
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		waitRunning(client.ObjectKeyFromObject(app), 1)

		expectWorkerRelease("widgets-oci", 1, 3)
		Expect(helmReleases(Default, "local", namespace)).Should(BeEmpty())
		By("the credentials stay on the hub")
		Expect(kerrors.IsNotFound(k8sClient.Get(workerCtx, types.NamespacedName{Namespace: namespace, Name: "registry-creds"}, &corev1.Secret{}))).Should(BeTrue())
	})

	It("installs only on the clusters a label selector matches", func() {
		const label = "e2e.kubevela.io/helmchart=spoke"
		_, err := execCommand("cluster", "labels", "add", WorkerClusterName, label)
		Expect(err).Should(Succeed())
		DeferCleanup(func() {
			_, err := execCommand("cluster", "labels", "del", WorkerClusterName, "e2e.kubevela.io/helmchart")
			Expect(err).Should(Succeed())
		})
		bs, err := json.Marshal(map[string]interface{}{"clusterLabelSelector": map[string]string{"e2e.kubevela.io/helmchart": "spoke"}})
		Expect(err).Should(Succeed())
		app := onWorker(widgetsComponent(widgetsURL, "", "widgets-selected", namespace, 8, ""))
		app.Spec.Policies = []v1beta1.AppPolicy{{Name: "spokes", Type: "topology", Properties: &runtime.RawExtension{Raw: bs}}}
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		waitRunning(client.ObjectKeyFromObject(app), 1)

		expectWorkerRelease("widgets-selected", 1, 8)
		Expect(helmReleases(Default, "local", namespace)).Should(BeEmpty())
		_, err = widget(hubCtx, "widgets-selected")
		// The hub has no Widget CRD, so the kind itself is unknown there.
		Expect(kerrors.IsNotFound(err) || meta.IsNoMatchError(err)).Should(BeTrue(), "the Widget on the hub: %v", err)
	})

	It("runs a chart's hooks in the release namespace on the hub too", func() {
		app := onWorker(widgetsComponent(widgetsURL, "", "widgets-hub", namespace, 5, ""))
		app.Spec.Policies = []v1beta1.AppPolicy{topologyPolicy("local")}
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			crd := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: widgetsCRD}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(hubCtx, crd))).Should(Succeed())
		})
		waitRunning(client.ObjectKeyFromObject(app), 1)

		Eventually(func(g Gomega) {
			rev, err := hookRevision(hubCtx, "widgets-hub")
			g.Expect(err).Should(Succeed())
			g.Expect(rev).Should(Equal("1"))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
		Expect(kerrors.IsNotFound(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: "vela-system", Name: "widgets-hub-hook"}, &corev1.ConfigMap{}))).Should(BeTrue())
	})

	It("is visible to helm on the worker's own API server, not only through cluster-gateway", func() {
		kubeconfig := directWorkerKubeconfig()
		if kubeconfig == "" {
			Skip("no kubeconfig reaches the worker directly from here; set E2E_WORKER_DIRECT_KUBECONFIG")
		}
		app := onWorker(widgetsComponent(widgetsURL, "", "widgets-direct", namespace, 4, ""))
		Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		waitRunning(client.ObjectKeyFromObject(app), 1)

		Eventually(func(g Gomega) {
			rel, ok := helmReleasesAt(g, kubeconfig, namespace)["widgets-direct"]
			g.Expect(ok).Should(BeTrue())
			g.Expect(rel.Info.Status).Should(Equal(release.StatusDeployed))
			g.Expect(rel.Chart.Metadata.Name).Should(Equal("widgets"))
		}).WithPolling(2 * time.Second).WithTimeout(helmTimeout).Should(Succeed())
	})
})
