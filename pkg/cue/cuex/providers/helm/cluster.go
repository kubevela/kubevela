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
	"time"

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

const (
	// clusterClientsCacheSize bounds the clusters whose clients are kept.
	clusterClientsCacheSize = 256
	// clusterClientsIdleTTL drops a cluster's clients, and its cached discovery, once unused this long.
	clusterClientsIdleTTL = time.Hour
)

// clusterClients is what every render for one cluster shares: the REST config that reaches it,
// acting as the controller, and an in-memory discovery cache. Helm invalidates that cache before
// each install or upgrade, when it reads the cluster's capabilities.
type clusterClients struct {
	config    *rest.Config
	discovery discovery.CachedDiscoveryInterface
}

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
// The local cluster with no user uses the Helm environment's own getter. Any other render gets a
// getter over its cluster's shared clients, with the user's impersonation on its own copy of the
// config, so discovery is cached per cluster however many users render there.
func (p *Provider) restClientGetter(ctx context.Context) (genericclioptions.RESTClientGetter, error) {
	cluster := clusterFrom(ctx)
	u, _ := request.UserFrom(ctx)
	if u != nil && u.GetName() == "" {
		u = nil
	}
	if pkgmulticluster.IsLocal(cluster) && u == nil {
		return p.helmClient.RESTClientGetter(), nil
	}
	clients, err := p.clientsFor(cluster)
	if err != nil {
		return nil, err
	}
	cfg := rest.CopyConfig(clients.config)
	if u != nil {
		cfg.Impersonate = rest.ImpersonationConfig{UserName: u.GetName(), Groups: u.GetGroups()}
	}
	return &clusterRESTClientGetter{config: cfg, discovery: clients.discovery}, nil
}

// clientsFor returns a cluster's shared clients, building them on first use. Building them makes
// no request, so holding the lock is cheap.
func (p *Provider) clientsFor(cluster string) (*clusterClients, error) {
	p.clusterClientsMu.Lock()
	defer p.clusterClientsMu.Unlock()
	if clients, ok := p.clusterClients.Get(cluster); ok {
		p.clusterClients.Put(cluster, clients, clusterClientsIdleTTL)
		return clients, nil
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
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build a discovery client for cluster %s", cluster)
	}
	clients := &clusterClients{config: cfg, discovery: memory.NewMemCacheClient(dc)}
	p.clusterClients.Put(cluster, clients, clusterClientsIdleTTL)
	return clients, nil
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

// clusterRESTClientGetter is a RESTClientGetter over a fixed REST config and a cluster's shared
// discovery. Each REST mapper it hands out is new, so it maps whatever discovery last fetched.
type clusterRESTClientGetter struct {
	config    *rest.Config
	discovery discovery.CachedDiscoveryInterface
}

// ToRESTConfig implements genericclioptions.RESTClientGetter.
func (g *clusterRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return rest.CopyConfig(g.config), nil
}

// ToDiscoveryClient implements genericclioptions.RESTClientGetter.
func (g *clusterRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return g.discovery, nil
}

// ToRESTMapper implements genericclioptions.RESTClientGetter.
func (g *clusterRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return restmapper.NewShortcutExpander(restmapper.NewDeferredDiscoveryRESTMapper(g.discovery), g.discovery, nil), nil
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
