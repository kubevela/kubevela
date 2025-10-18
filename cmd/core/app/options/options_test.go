/*
Copyright 2023 The KubeVela Authors.

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

package options

import (
	"testing"
	"time"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestCoreOptions_Flags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	// Use NewCoreOptions to properly initialize all config modules
	opt := NewCoreOptions()

	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	args := []string{
		"--application-re-sync-period=5s",
		"--cluster-metrics-interval=5s",
		"--enable-cluster-gateway=true",
		"--enable-cluster-metrics=true",
		"--enable-leader-election=true",
		"--health-addr=/healthz",
		"--informer-sync-period=3s",
		"--kube-api-burst=500",
		"--kube-api-qps=200",
		"--leader-election-lease-duration=3s",
		"--leader-election-namespace=test-namespace",
		"--leader-election-renew-deadline=5s",
		"--leader-election-retry-period=3s",
		"--log-debug=true",
		"--log-file-max-size=50",
		"--log-file-path=/path/to/log",
		"--max-dispatch-concurrent=5",
		"--max-workflow-failed-backoff-time=30",
		"--max-workflow-step-error-retry-times=5",
		"--max-workflow-wait-backoff-time=5",
		"--metrics-addr=/metrics",
		"--perf-enabled=true",
		"--use-webhook=true",
		"--webhook-cert-dir=/path/to/cert",
		"--webhook-port=8080",
	}

	if err := fs.Parse(args); err != nil {
		t.Errorf("Failed to parse args: %v", err)
	}

	// Verify specific flags were set correctly
	assert.Equal(t, true, opt.Webhook.UseWebhook)
	assert.Equal(t, "/path/to/cert", opt.Webhook.CertDir)
	assert.Equal(t, 8080, opt.Webhook.WebhookPort)
	assert.Equal(t, "/metrics", opt.Observability.MetricsAddr)
	assert.Equal(t, true, opt.Server.EnableLeaderElection)
	assert.Equal(t, "test-namespace", opt.Server.LeaderElectionNamespace)
	assert.Equal(t, "/path/to/log", opt.Observability.LogFilePath)
	assert.Equal(t, uint64(50), opt.Observability.LogFileMaxSize)
	assert.Equal(t, true, opt.Observability.LogDebug)
	assert.Equal(t, "/healthz", opt.Server.HealthAddr)
	assert.Equal(t, 3*time.Second, opt.Kubernetes.InformerSyncPeriod)
	assert.Equal(t, float64(200), opt.Kubernetes.QPS)
	assert.Equal(t, 500, opt.Kubernetes.Burst)
	assert.Equal(t, 3*time.Second, opt.Server.LeaseDuration)
	assert.Equal(t, 5*time.Second, opt.Server.RenewDeadline)
	assert.Equal(t, 3*time.Second, opt.Server.RetryPeriod)
	assert.Equal(t, true, opt.MultiCluster.EnableClusterGateway)
	assert.Equal(t, true, opt.MultiCluster.EnableClusterMetrics)
	assert.Equal(t, 5*time.Second, opt.MultiCluster.ClusterMetricsInterval)
}

func TestCuexOptions_Flags(t *testing.T) {
	pflag.NewFlagSet("test", pflag.ContinueOnError)
	cuex.EnableExternalPackageForDefaultCompiler = false
	cuex.EnableExternalPackageWatchForDefaultCompiler = false

	opts := NewCoreOptions()
	fss := opts.Flags()

	args := []string{
		"--enable-external-package-for-default-compiler=true",
		"--enable-external-package-watch-for-default-compiler=true",
	}
	err := fss.FlagSet("cue").Parse(args)
	if err != nil {
		return
	}

	// After parsing, the struct fields should be updated
	assert.True(t, opts.CUE.EnableExternalPackage, "The EnableExternalPackage field should be true after parsing")
	assert.True(t, opts.CUE.EnableExternalPackageWatch, "The EnableExternalPackageWatch field should be true after parsing")

	// Sync to globals to verify the sync mechanism works
	opts.CUE.SyncToCUEGlobals()
	assert.True(t, cuex.EnableExternalPackageForDefaultCompiler, "The --enable-external-package-for-default-compiler flag should be enabled after sync")
	assert.True(t, cuex.EnableExternalPackageWatchForDefaultCompiler, "The --enable-external-package-watch-for-default-compiler flag should be enabled after sync")
}
