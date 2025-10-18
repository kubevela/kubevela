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

	"github.com/spf13/pflag"
)

// KubernetesConfig contains Kubernetes API client configuration.
type KubernetesConfig struct {
	QPS                float64
	Burst              int
	InformerSyncPeriod time.Duration
}

// NewKubernetesConfig creates a new KubernetesConfig with defaults.
func NewKubernetesConfig() *KubernetesConfig {
	return &KubernetesConfig{
		QPS:                50,
		Burst:              100,
		InformerSyncPeriod: 10 * time.Hour,
	}
}

// AddFlags registers Kubernetes configuration flags.
func (c *KubernetesConfig) AddFlags(fs *pflag.FlagSet) {
	fs.Float64Var(&c.QPS, "kube-api-qps", c.QPS,
		"the qps for reconcile clients. Low qps may lead to low throughput. High qps may give stress to api-server. Raise this value if concurrent-reconciles is set to be high.")
	fs.IntVar(&c.Burst, "kube-api-burst", c.Burst,
		"the burst for reconcile clients. Recommend setting it qps*2.")
	fs.DurationVar(&c.InformerSyncPeriod, "informer-sync-period", c.InformerSyncPeriod,
		"The re-sync period for informer in controller-runtime. This is a system-level configuration.")
}
