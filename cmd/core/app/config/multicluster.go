/*
Copyright 2025 The KubeVela Authors.

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
	"time"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/spf13/pflag"
)

// MultiClusterConfig contains multi-cluster configuration.
type MultiClusterConfig struct {
	EnableClusterGateway   bool
	EnableClusterMetrics   bool
	ClusterMetricsInterval time.Duration
	// ClusterGatewayTLSSecret is the <namespace>/<name> of the Secret that holds
	// the cluster-gateway serving cert. If it is set and EnableClusterGateway is
	// true, vela-core creates or renews the cert at startup. vela-core writes
	// the CA into the cluster-gateway APIService when the APIService points at
	// ClusterGatewayService.
	ClusterGatewayTLSSecret string
	// ClusterGatewayService is the <namespace>/<name> of the Service that the
	// cluster-gateway serving cert is for. If it is set, vela-core sets the
	// caBundle of the cluster-gateway APIService only when the APIService points
	// at this Service. If it is empty, vela-core uses the Service of the
	// APIService.
	ClusterGatewayService string
}

// NewMultiClusterConfig creates a new MultiClusterConfig with defaults.
func NewMultiClusterConfig() *MultiClusterConfig {
	return &MultiClusterConfig{
		EnableClusterGateway:   false,
		EnableClusterMetrics:   false,
		ClusterMetricsInterval: 15 * time.Second,
	}
}

// AddFlags registers multi-cluster configuration flags.
func (c *MultiClusterConfig) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.EnableClusterGateway, "enable-cluster-gateway", c.EnableClusterGateway,
		"Enable cluster-gateway to use multicluster, disabled by default.")
	fs.BoolVar(&c.EnableClusterMetrics, "enable-cluster-metrics", c.EnableClusterMetrics,
		"Enable cluster-metrics-management to collect metrics from clusters with cluster-gateway, disabled by default. When this param is enabled, enable-cluster-gateway should be enabled")
	fs.DurationVar(&c.ClusterMetricsInterval, "cluster-metrics-interval", c.ClusterMetricsInterval,
		"The interval that ClusterMetricsMgr will collect metrics from clusters, default value is 15 seconds.")
	fs.StringVar(&c.ClusterGatewayTLSSecret, "cluster-gateway-tls-secret", c.ClusterGatewayTLSSecret,
		"The <namespace>/<name> of the Secret that holds the cluster-gateway serving cert. Requires --enable-cluster-gateway. If you set it, vela-core creates or renews the cert at startup and sets the caBundle of the cluster-gateway APIService when the APIService points at --cluster-gateway-service. If it is empty (the default), cert-manager or another tool must supply the cert.")
	fs.StringVar(&c.ClusterGatewayService, "cluster-gateway-service", c.ClusterGatewayService,
		"The <namespace>/<name> of the cluster-gateway Service that the serving cert is for. Use with --cluster-gateway-tls-secret. vela-core sets the APIService caBundle only when the APIService points at this Service. If it is empty, vela-core uses the Service of the APIService.")

	// Also register additional multicluster flags from external package
	pkgmulticluster.AddFlags(fs)
}
