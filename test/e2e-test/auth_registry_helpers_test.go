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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/registry"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	authTestNamespace = "kubevela-auth-test"
	authTestBearer    = "kubevela-auth-test-token"
	authTestUser      = "test-user"
	authTestPass      = "test-pass"
)

// setupAuthRegistries applies the static manifests under
// testdata/auth/manifests, materializes the runtime Secrets (registry-htpasswd,
// registry-tls) and ConfigMap (nginx-bearer-config) from committed files
// under testdata/auth/, and waits for the three registry Deployments to be
// Available.
func setupAuthRegistries(ctx context.Context, k8sClient client.Client) error {
	if err := applyManifestDir(ctx, k8sClient, "testdata/auth/manifests"); err != nil {
		return fmt.Errorf("applying auth registry manifests: %w", err)
	}
	if err := materializeAuthSecrets(ctx, k8sClient); err != nil {
		return err
	}
	return waitForAuthDeploymentsReady(ctx, k8sClient)
}

// pushTestChartToRegistries pushes testdata/auth/chart/podinfo-test-1.0.0.tgz
// to each registry using its native push protocol. Requires port-forwards to
// each registry Service from the test process.
func pushTestChartToRegistries(ctx context.Context, cfg *rest.Config) error {
	chartBytes, err := os.ReadFile("testdata/auth/chart/podinfo-test-1.0.0.tgz")
	if err != nil {
		return fmt.Errorf("reading test chart: %w", err)
	}
	if err := pushToChartMuseumBasic(ctx, cfg, chartBytes); err != nil {
		return fmt.Errorf("push to chartmuseum: %w", err)
	}
	if err := pushToChartMuseumBearer(ctx, cfg, chartBytes); err != nil {
		return fmt.Errorf("push to chartmuseum-bearer: %w", err)
	}
	if err := pushToZotOCI(ctx, cfg, chartBytes); err != nil {
		return fmt.Errorf("push to zot: %w", err)
	}
	return nil
}

// tearDownAuthRegistries deletes the kubevela-auth-test namespace. Garbage
// collection removes everything else.
func tearDownAuthRegistries(ctx context.Context, k8sClient client.Client) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: authTestNamespace}}
	return client.IgnoreNotFound(k8sClient.Delete(ctx, ns))
}

// --- runtime Secret + ConfigMap materialization ---

func materializeAuthSecrets(ctx context.Context, k8sClient client.Client) error {
	htpasswd, err := os.ReadFile("testdata/auth/htpasswd")
	if err != nil {
		return fmt.Errorf("reading htpasswd: %w", err)
	}
	crt, err := os.ReadFile("testdata/auth/certs/server.crt")
	if err != nil {
		return fmt.Errorf("reading server.crt: %w", err)
	}
	key, err := os.ReadFile("testdata/auth/certs/server.key")
	if err != nil {
		return fmt.Errorf("reading server.key: %w", err)
	}
	nginxConf, err := os.ReadFile("testdata/auth/nginx.conf")
	if err != nil {
		return fmt.Errorf("reading nginx.conf: %w", err)
	}

	objects := []client.Object{
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-htpasswd", Namespace: authTestNamespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"htpasswd": htpasswd},
		},
		// Opaque Secret with keys server.crt/server.key. The manifests
		// (chartmuseum, chartmuseum-bearer, zot) reference those exact
		// filenames via volumeMounts at /etc/certs/. kubernetes.io/tls
		// would force keys tls.crt/tls.key which don't match.
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-tls", Namespace: authTestNamespace},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"server.crt": crt, "server.key": key},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-bearer-config", Namespace: authTestNamespace},
			Data:       map[string]string{"nginx.conf": string(nginxConf)},
		},
	}
	for _, obj := range objects {
		if err := k8sClient.Create(ctx, obj); err != nil && !apierrIsAlreadyExists(err) {
			return fmt.Errorf("creating %T %s/%s: %w", obj, obj.GetNamespace(), obj.GetName(), err)
		}
	}
	return nil
}

// --- manifest application ---

func applyManifestDir(ctx context.Context, k8sClient client.Client, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	// namespace.yaml must apply first so subsequent objects can target it.
	sort := []string{"namespace.yaml"}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "namespace.yaml" || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		sort = append(sort, e.Name())
	}
	for _, name := range sort {
		if err := applyManifestFile(ctx, k8sClient, filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func applyManifestFile(ctx context.Context, k8sClient client.Client, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := yaml.NewYAMLOrJSONDecoder(bufio.NewReader(f), 4096)
	for {
		raw := map[string]interface{}{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if len(raw) == 0 {
			continue
		}
		obj, err := decodeKubeObject(raw)
		if err != nil {
			return err
		}
		if err := k8sClient.Create(ctx, obj); err != nil && !apierrIsAlreadyExists(err) {
			return err
		}
	}
}

func decodeKubeObject(raw map[string]interface{}) (client.Object, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	kind, _ := raw["kind"].(string)
	var obj client.Object
	switch kind {
	case "Namespace":
		obj = &corev1.Namespace{}
	case "ConfigMap":
		obj = &corev1.ConfigMap{}
	case "Secret":
		obj = &corev1.Secret{}
	case "Service":
		obj = &corev1.Service{}
	case "Deployment":
		obj = &appsv1.Deployment{}
	default:
		return nil, fmt.Errorf("decodeKubeObject: unsupported kind %q", kind)
	}
	if err := json.Unmarshal(b, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// --- ready-wait ---

func waitForAuthDeploymentsReady(ctx context.Context, k8sClient client.Client) error {
	for _, name := range []string{"zot", "chartmuseum", "chartmuseum-bearer"} {
		name := name
		gomega.Eventually(func() bool {
			d := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: authTestNamespace, Name: name}, d); err != nil {
				return false
			}
			for _, c := range d.Status.Conditions {
				if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
					return true
				}
			}
			return false
		}, 120*time.Second, 2*time.Second).Should(gomega.BeTrue(), "%s did not become Available", name)
	}
	return nil
}

// --- chart push helpers ---

func pushToChartMuseumBasic(ctx context.Context, cfg *rest.Config, chartBytes []byte) error {
	return withPortForward(cfg, "chartmuseum", 8080, func(localPort int) error {
		urlStr := fmt.Sprintf("https://127.0.0.1:%d/api/charts", localPort)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(chartBytes))
		if err != nil {
			return err
		}
		req.SetBasicAuth(authTestUser, authTestPass)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := insecureClient().Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusConflict {
			return nil
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("chartmuseum push: %s: %s", resp.Status, string(body))
		}
		return nil
	})
}

func pushToChartMuseumBearer(ctx context.Context, cfg *rest.Config, chartBytes []byte) error {
	return withPortForward(cfg, "chartmuseum-bearer", 443, func(localPort int) error {
		urlStr := fmt.Sprintf("https://127.0.0.1:%d/api/charts", localPort)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(chartBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+authTestBearer)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := insecureClient().Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusConflict {
			return nil
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("chartmuseum-bearer push: %s: %s", resp.Status, string(body))
		}
		return nil
	})
}

func pushToZotOCI(ctx context.Context, cfg *rest.Config, chartBytes []byte) error {
	return withPortForward(cfg, "zot", 5000, func(localPort int) error {
		host := fmt.Sprintf("127.0.0.1:%d", localPort)
		credFile, cleanup, err := writeZotCredsFile(host)
		if err != nil {
			return err
		}
		defer cleanup()
		client, err := registry.NewClient(
			registry.ClientOptCredentialsFile(credFile),
			registry.ClientOptPlainHTTP(),
		)
		if err != nil {
			return fmt.Errorf("create zot registry client: %w", err)
		}
		ref := host + "/charts/podinfo:1.0.0"
		if _, err := client.Push(chartBytes, ref); err != nil && !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "name unknown") && !strings.Contains(err.Error(), "BLOB_UNKNOWN") {
			return fmt.Errorf("zot push %s: %w", ref, err)
		}
		_ = ctx // ctx used by oras transport internally
		return nil
	})
}

func writeZotCredsFile(host string) (string, func(), error) {
	f, err := os.CreateTemp("", "kubevela-zot-creds-*.json")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	auth := base64.StdEncoding.EncodeToString([]byte(authTestUser + ":" + authTestPass))
	cfg := map[string]interface{}{
		"auths": map[string]interface{}{
			host: map[string]string{
				"username": authTestUser,
				"password": authTestPass,
				"auth":     auth,
			},
		},
	}
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return f.Name(), cleanup, nil
}

func insecureClient() *http.Client {
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
}

// --- port-forward plumbing ---

// withPortForward opens a port-forward to one pod of the named Service in
// the kubevela-auth-test namespace on remotePort, picks an ephemeral local
// port, runs fn with the local port, and tears down the forward.
func withPortForward(cfg *rest.Config, svcName string, remotePort int, fn func(localPort int) error) error {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	// Find a pod backing the Service by label selector "app=<svcName>".
	pods, err := clientset.CoreV1().Pods(authTestNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=" + svcName,
	})
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods for service %s", svcName)
	}
	podName := pods.Items[0].Name

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return err
	}
	pfURL := &url.URL{
		Scheme: "https",
		Host:   strings.TrimPrefix(cfg.Host, "https://"),
		Path:   fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", authTestNamespace, podName),
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, pfURL)

	localPort, err := freePort()
	if err != nil {
		return err
	}
	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	pf, err := portforward.New(
		dialer,
		[]string{fmt.Sprintf("%d:%d", localPort, remotePort)},
		stopCh, readyCh, out, errOut,
	)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- pf.ForwardPorts() }()
	select {
	case <-readyCh:
	case err := <-errCh:
		return fmt.Errorf("port-forward to %s:%d failed: %w (%s)", svcName, remotePort, err, errOut.String())
	case <-time.After(30 * time.Second):
		close(stopCh)
		return fmt.Errorf("port-forward to %s:%d timed out", svcName, remotePort)
	}
	defer close(stopCh)
	return fn(localPort)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// --- scheme registration helpers (used by callers that build their own rest.Config) ---

func authTestRestConfig() (*rest.Config, error) {
	kc := os.Getenv("KUBECONFIG")
	if kc == "" {
		kc = clientcmd.RecommendedHomeFile
	}
	return clientcmd.BuildConfigFromFlags("", kc)
}

// apierrIsAlreadyExists locally inlines the kerrors.IsAlreadyExists check
// to avoid bringing in another import just for one branch.
func apierrIsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}
