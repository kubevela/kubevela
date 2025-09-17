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

	"github.com/google/go-cmp/cmp"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	oamcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

func TestCoreOptions_Flags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opt := &CoreOptions{
		ControllerArgs: &oamcontroller.Args{},
	}

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

	expected := &CoreOptions{
		UseWebhook:              true,
		CertDir:                 "/path/to/cert",
		WebhookPort:             8080,
		MetricsAddr:             "/metrics",
		EnableLeaderElection:    true,
		LeaderElectionNamespace: "test-namespace",
		LogFilePath:             "/path/to/log",
		LogFileMaxSize:          50,
		LogDebug:                true,
		ControllerArgs:          &oamcontroller.Args{},
		HealthAddr:              "/healthz",
		StorageDriver:           "",
		InformerSyncPeriod:      3 * time.Second,
		QPS:                     200,
		Burst:                   500,
		LeaseDuration:           3 * time.Second,
		RenewDeadLine:           5 * time.Second,
		RetryPeriod:             3 * time.Second,
		EnableClusterGateway:    true,
		EnableClusterMetrics:    true,
		ClusterMetricsInterval:  5 * time.Second,
	}

	if !cmp.Equal(opt, expected, cmp.AllowUnexported(CoreOptions{})) {
		t.Errorf("Flags() diff: %v", cmp.Diff(opt, expected, cmp.AllowUnexported(CoreOptions{})))
	}
}

func TestCuexOptions_Flags(t *testing.T) {
	pflag.NewFlagSet("test", pflag.ContinueOnError)
	cuex.EnableExternalPackageForDefaultCompiler = false
	cuex.EnableExternalPackageWatchForDefaultCompiler = false

	opts := &CoreOptions{
		ControllerArgs: &oamcontroller.Args{},
	}
	fss := opts.Flags()

	args := []string{
		"--enable-external-package-for-default-compiler=true",
		"--enable-external-package-watch-for-default-compiler=true",
	}
	err := fss.FlagSet("generic").Parse(args)
	if err != nil {
		return
	}

	assert.True(t, cuex.EnableExternalPackageForDefaultCompiler, "The --enable-external-package-for-default-compiler flag should be enabled")
	assert.True(t, cuex.EnableExternalPackageWatchForDefaultCompiler, "The --enable-external-package-watch-for-default-compiler flag should be enabled")
}

func TestNewCoreOptions(t *testing.T) {
	opts := NewCoreOptions()

	assert.NotNil(t, opts)
	assert.False(t, opts.UseWebhook)
	assert.Equal(t, "/k8s-webhook-server/serving-certs", opts.CertDir)
	assert.Equal(t, 9443, opts.WebhookPort)
	assert.Equal(t, ":8080", opts.MetricsAddr)
	assert.False(t, opts.EnableLeaderElection)
	assert.Equal(t, "", opts.LeaderElectionNamespace)
	assert.Equal(t, "", opts.LogFilePath)
	assert.Equal(t, uint64(1024), opts.LogFileMaxSize)
	assert.False(t, opts.LogDebug)
	assert.NotNil(t, opts.ControllerArgs)
	assert.Equal(t, ":9440", opts.HealthAddr)
	assert.Equal(t, "Local", opts.StorageDriver)
	assert.Equal(t, 10*time.Hour, opts.InformerSyncPeriod)
	assert.Equal(t, 50.0, opts.QPS)
	assert.Equal(t, 100, opts.Burst)
	assert.Equal(t, 15*time.Second, opts.LeaseDuration)
	assert.Equal(t, 10*time.Second, opts.RenewDeadLine)
	assert.Equal(t, 2*time.Second, opts.RetryPeriod)
	assert.False(t, opts.EnableClusterGateway)
	assert.False(t, opts.EnableClusterMetrics)
	assert.Equal(t, 15*time.Second, opts.ClusterMetricsInterval)
}

func TestFlags_AllFlagSets(t *testing.T) {
	opts := NewCoreOptions()
	fss := opts.Flags()

	// Test that all expected flag sets are present
	expectedFlagSets := []string{
		"generic", "controllerArgs", "commonconfig", "oam", "optimize",
		"admission", "resourcekeeper", "wfTypes", "multicluster",
		"controllerreconciles", "featuregate", "sharding", "klog",
		"controllerclient", "profiling",
	}

	for _, fsName := range expectedFlagSets {
		flagSet := fss.FlagSet(fsName)
		assert.NotNil(t, flagSet, "Flag set %s should exist", fsName)
	}
}

func TestFlags_DefaultValues(t *testing.T) {
	opts := NewCoreOptions()
	fss := opts.Flags()

	// Test that flags are set up with correct defaults
	genericFS := fss.FlagSet("generic")
	assert.NotNil(t, genericFS)

	// Verify some key flags exist
	useWebhookFlag := genericFS.Lookup("use-webhook")
	assert.NotNil(t, useWebhookFlag)
	assert.Equal(t, "false", useWebhookFlag.DefValue)

	certDirFlag := genericFS.Lookup("webhook-cert-dir")
	assert.NotNil(t, certDirFlag)
	assert.Equal(t, "/k8s-webhook-server/serving-certs", certDirFlag.DefValue)

	metricsAddrFlag := genericFS.Lookup("metrics-addr")
	assert.NotNil(t, metricsAddrFlag)
	assert.Equal(t, ":8080", metricsAddrFlag.DefValue)
}

func TestFlags_LoggingConfiguration(t *testing.T) {
	opts := NewCoreOptions()
	fss := opts.Flags()

	// Test the klog flagset for logging configuration
	klogFS := fss.FlagSet("klog")
	assert.NotNil(t, klogFS)

	// Test LogDebug flag
	opts.LogDebug = true
	fss = opts.Flags()
	klogFS = fss.FlagSet("klog")
	// The LogDebug flag should set the verbosity level
	vFlag := klogFS.Lookup("v")
	assert.NotNil(t, vFlag)

	// Test LogFilePath flag
	opts.LogFilePath = "/var/log/vela.log"
	opts.LogFileMaxSize = 2048
	fss = opts.Flags()
	klogFS = fss.FlagSet("klog")

	// Check that log file flags are configured
	logFileFlag := klogFS.Lookup("log_file")
	assert.NotNil(t, logFileFlag)

	logFileMaxSizeFlag := klogFS.Lookup("log_file_max_size")
	assert.NotNil(t, logFileMaxSizeFlag)

	// Test setting the flags
	err := klogFS.Set("logtostderr", "false")
	assert.NoError(t, err)

	err = klogFS.Set("log_file", opts.LogFilePath)
	assert.NoError(t, err)

	err = klogFS.Set("log_file_max_size", "2048")
	assert.NoError(t, err)
}
