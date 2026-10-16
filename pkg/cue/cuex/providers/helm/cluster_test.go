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

package helm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/kubevela/pkg/cue/cuex/providers"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/singleton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// pathRecorder is an API server stub that answers /version on any prefix and records each
// path and the user each request impersonates.
type pathRecorder struct {
	mu    sync.Mutex
	paths []string
	users []string
}

func (r *pathRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.paths = append(r.paths, req.URL.Path)
	r.users = append(r.users, req.Header.Get(transport.ImpersonateUserHeader))
	r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"major":"1","minor":"31","gitVersion":"v1.31.0"}`))
}

func (r *pathRecorder) Paths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func (r *pathRecorder) Users() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.users...)
}

// renderCtx is the context the controller renders a component in: its cluster, and the
// Application's user when there is one.
func renderCtx(cluster string, u user.Info) context.Context {
	ctx := pkgmulticluster.WithCluster(context.Background(), cluster)
	if u != nil {
		ctx = request.WithUser(ctx, u)
	}
	return ctx
}

var _ = Describe("Helm clients per cluster", func() {
	const proxyVersion = "/apis/cluster.core.oam.dev/v1alpha1/clustergateways/managed/proxy/version"

	var (
		hub     *pathRecorder
		hubSrv  *httptest.Server
		p       *Provider
		saveOpt pkgmulticluster.ClusterGatewayClientOptions
	)

	BeforeEach(func() {
		hub = &pathRecorder{}
		hubSrv = httptest.NewServer(hub)
		p = NewProviderWithConfig(nil)
		p.hubConfig = func() (*rest.Config, error) { return &rest.Config{Host: hubSrv.URL}, nil }
		saveOpt = pkgmulticluster.DefaultClusterGatewayClientOptions
	})

	AfterEach(func() {
		pkgmulticluster.DefaultClusterGatewayClientOptions = saveOpt
		hubSrv.Close()
		p.Close()
	})

	// serverVersion makes a request with the getter's REST config, the one Helm's writes use.
	serverVersion := func(ctx context.Context) {
		getter, err := p.restClientGetter(ctx)
		Expect(err).ShouldNot(HaveOccurred())
		cfg, err := getter.ToRESTConfig()
		Expect(err).ShouldNot(HaveOccurred())
		dc, err := discovery.NewDiscoveryClientForConfig(cfg)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = dc.ServerVersion()
		Expect(err).ShouldNot(HaveOccurred())
	}

	It("uses the Helm environment's own client for the local cluster with no user", func() {
		for _, ctx := range []context.Context{context.Background(), renderCtx(pkgmulticluster.Local, nil), renderCtx(pkgmulticluster.Local, &user.DefaultInfo{})} {
			getter, err := p.restClientGetter(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(getter).Should(BeIdenticalTo(p.helmClient.RESTClientGetter()))
		}
	})

	It("acts as the render's user on the local cluster", func() {
		serverVersion(renderCtx(pkgmulticluster.Local, &user.DefaultInfo{Name: "alice", Groups: []string{"dev"}}))
		Expect(hub.Paths()).Should(ConsistOf("/version"))
		Expect(hub.Users()).Should(ConsistOf("alice"))
	})

	It("acts as the render's user on a managed cluster", func() {
		serverVersion(renderCtx("managed", &user.DefaultInfo{Name: "alice"}))
		Expect(hub.Paths()).Should(ConsistOf(proxyVersion))
		Expect(hub.Users()).Should(ConsistOf("alice"))
	})

	It("sends a managed cluster's requests through the hub's cluster-gateway proxy", func() {
		serverVersion(renderCtx("managed", nil))
		Expect(hub.Paths()).Should(ConsistOf(proxyVersion))
		Expect(hub.Users()).Should(ConsistOf(""))
	})

	It("sends a managed cluster's requests straight to cluster-gateway when its URL is set", func() {
		gw := &pathRecorder{}
		gwSrv := httptest.NewServer(gw)
		defer gwSrv.Close()
		pkgmulticluster.DefaultClusterGatewayClientOptions = pkgmulticluster.ClusterGatewayClientOptions{URL: gwSrv.URL}

		serverVersion(renderCtx("managed", nil))
		Expect(gw.Paths()).Should(ConsistOf(proxyVersion))
		Expect(hub.Paths()).Should(BeEmpty())
	})

	It("shares a cluster's discovery between its users, and keeps it apart from other clusters'", func() {
		discoveryOf := func(ctx context.Context) interface{} {
			g, err := p.restClientGetter(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			dc, err := g.ToDiscoveryClient()
			Expect(err).ShouldNot(HaveOccurred())
			return dc
		}
		a := discoveryOf(renderCtx("a", nil))
		Expect(discoveryOf(renderCtx("a", &user.DefaultInfo{Name: "alice"}))).Should(BeIdenticalTo(a))
		Expect(discoveryOf(renderCtx("a", &user.DefaultInfo{Name: "bob", Groups: []string{"ops"}}))).Should(BeIdenticalTo(a))
		Expect(discoveryOf(renderCtx("b", nil))).ShouldNot(BeIdenticalTo(a))
	})

	It("reports the Kubernetes version of the cluster being rendered for", func() {
		kv := p.getKubeVersion(renderCtx("managed", nil))
		Expect(kv).ShouldNot(BeNil())
		Expect(kv.Version).Should(Equal("v1.31.0"))
		Expect(hub.Paths()).Should(ConsistOf(proxyVersion))
	})

	It("keys the release cache by cluster", func() {
		Expect(releaseCacheKey("a", "ns", "r")).ShouldNot(Equal(releaseCacheKey("b", "ns", "r")))
		Expect(releaseCacheKey("", "ns", "r")).Should(Equal(releaseCacheKey(pkgmulticluster.Local, "ns", "r")))
	})

	It("installs into the cluster named by the render context", func() {
		var configClusters, clientClusters []string
		cfg := fakeActionConfig()
		cs := clientsetfake.NewSimpleClientset()
		p.actionConfigFactory = func(ctx context.Context, _ string) (*action.Configuration, error) {
			configClusters = append(configClusters, clusterFrom(ctx))
			return cfg, nil
		}
		p.kubeClientFactory = func(ctx context.Context) (kubernetes.Interface, error) {
			clientClusters = append(clientClusters, clusterFrom(ctx))
			return cs, nil
		}

		ctx := renderCtx("managed", nil)
		_, _, _, err := p.installOrUpgradeChart(ctx, minimalChart("c", "1.0.0"), "r", "ns", map[string]interface{}{}, nil, nil)
		Expect(err).ShouldNot(HaveOccurred())
		By("adopting the release, which labels its Secrets through the Kubernetes client")
		velaCtx := &ContextParams{AppName: "app", AppNamespace: "ns", Name: "comp"}
		_, _, _, err = p.installOrUpgradeChart(ctx, minimalChart("c", "1.0.0"), "r", "ns", map[string]interface{}{}, nil, velaCtx)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(configClusters).ShouldNot(BeEmpty())
		Expect(configClusters).Should(HaveEach("managed"))
		Expect(clientClusters).Should(HaveEach("managed"))
		Expect(p.releaseVersions).Should(HaveKey(releaseCacheKey("managed", "ns", "r")))
	})
})

var _ = Describe("Render targets the component's cluster and user", func() {
	It("installs on the cluster the template names, as the Application's user", func() {
		scheme := runtime.NewScheme()
		Expect(v1beta1.AddToScheme(scheme)).To(Succeed())
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{
			Name: "shop", Namespace: "team-a",
			Annotations: map[string]string{oam.AnnotationApplicationUsername: "alice"},
		}}
		origKube := singleton.KubeClient.Get()
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build())
		defer singleton.KubeClient.Set(origKube)

		p := NewProvider()
		saveConfig, saveClient := p.actionConfigFactory, p.kubeClientFactory
		defer func() { p.actionConfigFactory, p.kubeClientFactory = saveConfig, saveClient }()
		var clusters, users []string
		record := func(ctx context.Context) {
			clusters = append(clusters, clusterFrom(ctx))
			u, _ := request.UserFrom(ctx)
			Expect(u).ShouldNot(BeNil())
			users = append(users, u.GetName())
		}
		cfg := fakeActionConfig()
		cs := clientsetfake.NewSimpleClientset()
		p.actionConfigFactory = func(ctx context.Context, _ string) (*action.Configuration, error) {
			record(ctx)
			return cfg, nil
		}
		p.kubeClientFactory = func(ctx context.Context) (kubernetes.Interface, error) {
			record(ctx)
			return cs, nil
		}
		p.cache.Put("repo/"+sourceCacheID("render-target")+"/1.0.0", createChartArchive(&chart.Chart{
			Metadata: &chart.Metadata{Name: "render-target", Version: "1.0.0"},
			Templates: []*chart.File{{
				Name: "templates/cm.yaml",
				Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}-cm\n"),
			}},
		}), time.Hour)

		_, err := Render(context.Background(), &providers.Params[RenderParams]{Params: RenderParams{
			Chart:   ChartSourceParams{Source: "render-target", Version: "1.0.0"},
			Release: &ReleaseParams{Name: "rel", Namespace: "team-a"},
			Context: &ContextParams{AppName: "shop", AppNamespace: "team-a", Name: "web", Namespace: "team-a", Cluster: "eu-west"},
		}})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(clusters).ShouldNot(BeEmpty())
		Expect(clusters).Should(HaveEach("eu-west"))
		Expect(users).Should(HaveEach("alice"))
	})
})

var _ = Describe("Helm action configs", func() {
	It("place objects that name no namespace, hooks included, in the release namespace", func() {
		p := NewProviderWithConfig(nil)
		defer p.Close()
		p.hubConfig = func() (*rest.Config, error) { return &rest.Config{Host: "https://127.0.0.1:1"}, nil }
		for _, ctx := range []context.Context{context.Background(), renderCtx("managed", nil)} {
			getter, err := p.restClientGetter(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			ns, explicit, err := inReleaseNamespace(getter, "team-a").ToRawKubeConfigLoader().Namespace()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(explicit).Should(BeTrue())
			Expect(ns).Should(Equal("team-a"))
		}
	})
})

var _ = Describe("Dry-run renders", func() {
	It("track no release Secrets, since they install nothing", func() {
		p := NewProvider()
		saveClient, saveServer := p.kubeClientFactory, p.helmClient.KubeAPIServer
		defer func() { p.kubeClientFactory, p.helmClient.KubeAPIServer = saveClient, saveServer }()
		p.helmClient.KubeAPIServer = "https://127.0.0.1:1"
		p.kubeClientFactory = func(context.Context) (kubernetes.Interface, error) {
			Fail("a dry-run must not read the cluster's release Secrets")
			return nil, nil
		}
		p.cache.Put("repo/"+sourceCacheID("dryrun-secrets")+"/1.0.0", createChartArchive(&chart.Chart{
			Metadata:  &chart.Metadata{Name: "dryrun-secrets", Version: "1.0.0"},
			Templates: []*chart.File{{Name: "templates/cm.yaml", Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n")}},
		}), time.Hour)

		result, err := Render(WithDryRun(context.Background()), &providers.Params[RenderParams]{Params: RenderParams{
			Chart:   ChartSourceParams{Source: "dryrun-secrets", Version: "1.0.0"},
			Release: &ReleaseParams{Name: "rel", Namespace: "team-a"},
			Context: &ContextParams{AppName: "shop", AppNamespace: "team-a", Name: "web", Namespace: "team-a"},
		}})
		Expect(err).ShouldNot(HaveOccurred())
		for _, r := range result.Returns.Resources {
			Expect(r["kind"]).ShouldNot(Equal("Secret"))
		}
	})

	It("check charts and templates against client-go's Kubernetes version when the cluster's cannot be read", func() {
		p := NewProviderWithConfig(nil)
		defer p.Close()
		p.helmClient.KubeAPIServer = "https://127.0.0.1:1"
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "needs-new-k8s", Version: "1.0.0", KubeVersion: ">=1.29.0-0"},
			Templates: []*chart.File{{Name: "templates/cm.yaml", Data: []byte(
				"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\ndata:\n  minor: \"{{ .Capabilities.KubeVersion.Minor }}\"\n")}},
		}
		Expect(p.getKubeVersion(context.Background())).Should(BeNil())
		fallback := fallbackKubeVersion()
		Expect(fallback).ShouldNot(BeNil())

		manifest, _, err := p.dryRunRender(context.Background(), ch, "rel", "team-a", map[string]interface{}{}, nil, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(manifest).Should(ContainSubstring(`minor: "` + fallback.Minor + `"`))
	})
})

var _ = Describe("The cluster client store", func() {
	It("is bounded by cluster, dropping the least recently used", func() {
		p := NewProviderWithConfig(nil)
		defer p.Close()
		p.hubConfig = func() (*rest.Config, error) { return &rest.Config{Host: "https://127.0.0.1:1"}, nil }
		first, err := p.clientsFor("c-0")
		Expect(err).ShouldNot(HaveOccurred())
		for i := 1; i <= clusterClientsCacheSize; i++ {
			_, err := p.clientsFor(fmt.Sprintf("c-%d", i))
			Expect(err).ShouldNot(HaveOccurred())
		}
		again, err := p.clientsFor("c-0")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(again).ShouldNot(BeIdenticalTo(first))
	})

	It("maps a kind created after the cluster's first render, once Helm invalidates discovery", func() {
		api := &discoveryStub{}
		srv := httptest.NewServer(api)
		defer srv.Close()
		p := NewProviderWithConfig(nil)
		defer p.Close()
		p.hubConfig = func() (*rest.Config, error) { return &rest.Config{Host: srv.URL}, nil }
		widget := schema.GroupKind{Group: "e2e.kubevela.io", Kind: "Widget"}
		mapping := func() error {
			getter, err := p.restClientGetter(renderCtx("managed", nil))
			Expect(err).ShouldNot(HaveOccurred())
			mapper, err := getter.ToRESTMapper()
			Expect(err).ShouldNot(HaveOccurred())
			_, err = mapper.RESTMapping(widget, "v1")
			return err
		}
		Expect(meta.IsNoMatchError(mapping())).Should(BeTrue())

		api.addWidgets()
		getter, err := p.restClientGetter(renderCtx("managed", nil))
		Expect(err).ShouldNot(HaveOccurred())
		dc, err := getter.ToDiscoveryClient()
		Expect(err).ShouldNot(HaveOccurred())
		// What Helm's getCapabilities does before each install or upgrade: invalidate, then refetch.
		dc.Invalidate()
		_, _, err = dc.ServerGroupsAndResources()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(mapping()).Should(Succeed())
	})
})

// discoveryStub serves legacy discovery for core/v1 and, once added, e2e.kubevela.io/v1 widgets,
// on any cluster-gateway proxy prefix.
type discoveryStub struct {
	mu      sync.Mutex
	widgets bool
}

func (d *discoveryStub) addWidgets() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.widgets = true
}

func (d *discoveryStub) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	d.mu.Lock()
	widgets := d.widgets
	d.mu.Unlock()
	path := req.URL.Path
	if i := strings.Index(path, "/proxy/"); i >= 0 {
		path = path[i+len("/proxy"):]
	}
	w.Header().Set("Content-Type", "application/json")
	switch path {
	case "/api":
		_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":["v1"]}`))
	case "/api/v1":
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["get"]}]}`))
	case "/apis":
		groups := `[]`
		if widgets {
			groups = `[{"name":"e2e.kubevela.io","versions":[{"groupVersion":"e2e.kubevela.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"e2e.kubevela.io/v1","version":"v1"}}]`
		}
		_, _ = w.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":` + groups + `}`))
	case "/apis/e2e.kubevela.io/v1":
		if !widgets {
			http.NotFound(w, req)
			return
		}
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","groupVersion":"e2e.kubevela.io/v1","resources":[{"name":"widgets","namespaced":true,"kind":"Widget","verbs":["get"]}]}`))
	default:
		http.NotFound(w, req)
	}
}
