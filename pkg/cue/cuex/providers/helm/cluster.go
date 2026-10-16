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

// Kubernetes clients for the cluster a chart is rendered for, acting as the render's user: the hub
// for the local cluster, and cluster-gateway for a managed one, routed the way the controller's own
// multicluster client is.

import (
	"context"
	"os"
	"strings"
	"sync"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// clusterFrom returns the cluster a render is for, "local" when the context names none.
func clusterFrom(ctx context.Context) string {
	if cluster, ok := pkgmulticluster.ClusterFrom(ctx); ok && !pkgmulticluster.IsLocal(cluster) {
		return cluster
	}
	return pkgmulticluster.Local
}

// restClientGetter returns the Helm client getter for the cluster a render is for, acting as the
// user the render carries, as the controller's impersonating round tripper does. Helm does not
// pass the caller's context on every request, so both are pinned on the getter's config.
//
// The local cluster with no user uses the Helm environment's own getter. Every other cluster and
// user keeps one getter, so discovery and the REST mapper are cached per getter: every proxied
// cluster shares the hub's host, which is what client-go keys its own caches by.
func (p *Provider) restClientGetter(ctx context.Context) (genericclioptions.RESTClientGetter, error) {
	cluster := clusterFrom(ctx)
	u, _ := request.UserFrom(ctx)
	if u != nil && u.GetName() == "" {
		u = nil
	}
	if pkgmulticluster.IsLocal(cluster) && u == nil {
		return p.helmClient.RESTClientGetter(), nil
	}
	key := cluster
	if u != nil {
		key += "\x00" + u.GetName() + "\x00" + strings.Join(u.GetGroups(), ",")
	}
	p.clusterGettersMu.Lock()
	defer p.clusterGettersMu.Unlock()
	if getter, ok := p.clusterGetters[key]; ok {
		return getter, nil
	}
	hub, err := p.hubConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the hub REST config")
	}
	cfg := rest.CopyConfig(hub)
	if !pkgmulticluster.IsLocal(cluster) {
		if cfg, err = routeToCluster(cfg, cluster); err != nil {
			return nil, err
		}
	}
	if u != nil {
		cfg.Impersonate = rest.ImpersonationConfig{UserName: u.GetName(), Groups: u.GetGroups()}
	}
	getter := &clusterRESTClientGetter{config: cfg}
	p.clusterGetters[key] = getter
	return getter, nil
}

// routeToCluster returns a copy of the hub config that reaches a managed cluster through
// cluster-gateway: the aggregated API by default, or the gateway itself when
// --cluster-gateway-url is set, as kubevela/pkg's multicluster.NewClient does.
func routeToCluster(hub *rest.Config, cluster string) (*rest.Config, error) {
	cfg := rest.CopyConfig(hub)
	cfg.Wrap(pkgmulticluster.NewTransportWrapper(pkgmulticluster.ForCluster(cluster)))
	gateway := pkgmulticluster.DefaultClusterGatewayClientOptions
	if gateway.URL == "" {
		return cfg, nil
	}
	cfg.Host = gateway.URL
	// client-go rejects a CA file alongside Insecure, so the hub's in-cluster CA file goes either way.
	cfg.CAFile = ""
	if gateway.CAFile == "" {
		cfg.CAData = nil
		cfg.Insecure = true
		return cfg, nil
	}
	ca, err := os.ReadFile(gateway.CAFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the cluster-gateway CA file")
	}
	cfg.CAData = ca
	return cfg, nil
}

// clusterRESTClientGetter is a RESTClientGetter over a fixed REST config, with in-memory
// discovery that Helm invalidates itself after installing CRDs.
type clusterRESTClientGetter struct {
	config *rest.Config

	once      sync.Once
	discovery discovery.CachedDiscoveryInterface
	mapper    meta.RESTMapper
	err       error
}

func (g *clusterRESTClientGetter) init() {
	g.once.Do(func() {
		dc, err := discovery.NewDiscoveryClientForConfig(g.config)
		if err != nil {
			g.err = err
			return
		}
		g.discovery = memory.NewMemCacheClient(dc)
		g.mapper = restmapper.NewShortcutExpander(restmapper.NewDeferredDiscoveryRESTMapper(g.discovery), g.discovery, nil)
	})
}

// ToRESTConfig implements genericclioptions.RESTClientGetter.
func (g *clusterRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.CopyConfig(g.config), nil
}

// ToDiscoveryClient implements genericclioptions.RESTClientGetter.
func (g *clusterRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	g.init()
	return g.discovery, g.err
}

// ToRESTMapper implements genericclioptions.RESTClientGetter.
func (g *clusterRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	g.init()
	return g.mapper, g.err
}

// ToRawKubeConfigLoader implements genericclioptions.RESTClientGetter. Helm reads only the
// namespace from it, and inReleaseNamespace overrides that.
func (g *clusterRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), &clientcmd.ConfigOverrides{})
}

// inReleaseNamespace wraps a getter so its kubeconfig reports the release namespace. Helm's kube
// client places every object that names no namespace there, and the post-renderer that stamps
// the namespace on rendered resources never sees hooks.
func inReleaseNamespace(getter genericclioptions.RESTClientGetter, namespace string) genericclioptions.RESTClientGetter {
	return &namespacedGetter{RESTClientGetter: getter, namespace: namespace}
}

type namespacedGetter struct {
	genericclioptions.RESTClientGetter
	namespace string
}

// ToRawKubeConfigLoader implements genericclioptions.RESTClientGetter.
func (g *namespacedGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return &namespacedClientConfig{base: g.RESTClientGetter.ToRawKubeConfigLoader(), namespace: g.namespace}
}

// namespacedClientConfig is a clientcmd.ClientConfig that reports a fixed namespace.
type namespacedClientConfig struct {
	base      clientcmd.ClientConfig
	namespace string
}

// RawConfig implements clientcmd.ClientConfig.
func (c *namespacedClientConfig) RawConfig() (clientcmdapi.Config, error) { return c.base.RawConfig() }

// ClientConfig implements clientcmd.ClientConfig.
func (c *namespacedClientConfig) ClientConfig() (*rest.Config, error) { return c.base.ClientConfig() }

// Namespace implements clientcmd.ClientConfig.
func (c *namespacedClientConfig) Namespace() (string, bool, error) { return c.namespace, true, nil }

// ConfigAccess implements clientcmd.ClientConfig.
func (c *namespacedClientConfig) ConfigAccess() clientcmd.ConfigAccess { return c.base.ConfigAccess() }
