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
	wfTypes "github.com/kubevela/workflow/pkg/types"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

func TestNewCoreOptions_DefaultValues(t *testing.T) {
	opt := NewCoreOptions()

	// Test Server defaults
	assert.Equal(t, ":9440", opt.Server.HealthAddr)
	assert.Equal(t, "Local", opt.Server.StorageDriver)
	assert.Equal(t, false, opt.Server.EnableLeaderElection)
	assert.Equal(t, "", opt.Server.LeaderElectionNamespace)
	assert.Equal(t, 15*time.Second, opt.Server.LeaseDuration)
	assert.Equal(t, 10*time.Second, opt.Server.RenewDeadline)
	assert.Equal(t, 2*time.Second, opt.Server.RetryPeriod)

	// Test Webhook defaults
	assert.Equal(t, false, opt.Webhook.UseWebhook)
	assert.Equal(t, "/k8s-webhook-server/serving-certs", opt.Webhook.CertDir)
	assert.Equal(t, 9443, opt.Webhook.WebhookPort)

	// Test Observability defaults
	assert.Equal(t, ":8080", opt.Observability.MetricsAddr)
	assert.Equal(t, false, opt.Observability.LogDebug)
	assert.Equal(t, "", opt.Observability.LogFilePath)
	assert.Equal(t, uint64(1024), opt.Observability.LogFileMaxSize)

	// Test Kubernetes defaults
	assert.Equal(t, 10*time.Hour, opt.Kubernetes.InformerSyncPeriod)
	assert.Equal(t, float64(50), opt.Kubernetes.QPS)
	assert.Equal(t, 100, opt.Kubernetes.Burst)

	// Test MultiCluster defaults
	assert.Equal(t, false, opt.MultiCluster.EnableClusterGateway)
	assert.Equal(t, false, opt.MultiCluster.EnableClusterMetrics)
	assert.Equal(t, 15*time.Second, opt.MultiCluster.ClusterMetricsInterval)

	// Test CUE defaults
	assert.NotNil(t, opt.CUE)

	// Test Application defaults
	assert.Equal(t, 5*time.Minute, opt.Application.ReSyncPeriod)

	// Test OAM defaults
	assert.Equal(t, "vela-system", opt.OAM.SystemDefinitionNamespace)

	// Test Performance defaults
	assert.Equal(t, false, opt.Performance.PerfEnabled)

	// Test Controller defaults
	assert.Equal(t, 50, opt.Controller.RevisionLimit)
	assert.Equal(t, 10, opt.Controller.AppRevisionLimit)
	assert.Equal(t, 20, opt.Controller.DefRevisionLimit)
	assert.Equal(t, true, opt.Controller.AutoGenWorkloadDefinition)
	assert.Equal(t, 4, opt.Controller.ConcurrentReconciles)
	assert.Equal(t, false, opt.Controller.IgnoreAppWithoutControllerRequirement)
	assert.Equal(t, false, opt.Controller.IgnoreDefinitionWithoutControllerRequirement)

	// Test Workflow defaults
	assert.Equal(t, 60, opt.Workflow.MaxWaitBackoffTime)
	assert.Equal(t, 300, opt.Workflow.MaxFailedBackoffTime)
	assert.Equal(t, 10, opt.Workflow.MaxStepErrorRetryTimes)

	// Test Resource defaults
	assert.Equal(t, 10, opt.Resource.MaxDispatchConcurrent)

	// Ensure all config modules are initialized
	assert.NotNil(t, opt.Admission)
	assert.NotNil(t, opt.Client)
	assert.NotNil(t, opt.Reconcile)
	assert.NotNil(t, opt.Sharding)
	assert.NotNil(t, opt.Feature)
	assert.NotNil(t, opt.Profiling)
	assert.NotNil(t, opt.KLog)
	assert.NotNil(t, opt.Controller)
}

func TestCoreOptions_FlagsCompleteSet(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opt := NewCoreOptions()

	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	args := []string{
		// Server flags
		"--health-addr=/healthz",
		"--storage-driver=MongoDB",
		"--enable-leader-election=true",
		"--leader-election-namespace=test-namespace",
		"--leader-election-lease-duration=3s",
		"--leader-election-renew-deadline=5s",
		"--leader-election-retry-period=3s",
		// Webhook flags
		"--use-webhook=true",
		"--webhook-cert-dir=/path/to/cert",
		"--webhook-port=8080",
		// Observability flags
		"--metrics-addr=/metrics",
		"--log-debug=true",
		"--log-file-path=/path/to/log",
		"--log-file-max-size=50",
		// Kubernetes flags
		"--informer-sync-period=3s",
		"--kube-api-qps=200",
		"--kube-api-burst=500",
		// MultiCluster flags
		"--enable-cluster-gateway=true",
		"--enable-cluster-metrics=true",
		"--cluster-metrics-interval=5s",
		// CUE flags
		"--enable-external-package-for-default-compiler=true",
		"--enable-external-package-watch-for-default-compiler=true",
		// Application flags
		"--application-re-sync-period=5s",
		// OAM flags
		"--system-definition-namespace=custom-namespace",
		// Performance flags
		"--perf-enabled=true",
		// Controller flags
		"--revision-limit=100",
		"--application-revision-limit=20",
		"--definition-revision-limit=30",
		"--autogen-workload-definition=false",
		"--concurrent-reconciles=8",
		"--ignore-app-without-controller-version=true",
		"--ignore-definition-without-controller-version=true",
		// Workflow flags
		"--max-workflow-wait-backoff-time=30",
		"--max-workflow-failed-backoff-time=150",
		"--max-workflow-step-error-retry-times=5",
		// Resource flags
		"--max-dispatch-concurrent=5",
	}

	err := fs.Parse(args)
	require.NoError(t, err)

	// Verify Server flags
	assert.Equal(t, "/healthz", opt.Server.HealthAddr)
	assert.Equal(t, "MongoDB", opt.Server.StorageDriver)
	assert.Equal(t, true, opt.Server.EnableLeaderElection)
	assert.Equal(t, "test-namespace", opt.Server.LeaderElectionNamespace)
	assert.Equal(t, 3*time.Second, opt.Server.LeaseDuration)
	assert.Equal(t, 5*time.Second, opt.Server.RenewDeadline)
	assert.Equal(t, 3*time.Second, opt.Server.RetryPeriod)

	// Verify Webhook flags
	assert.Equal(t, true, opt.Webhook.UseWebhook)
	assert.Equal(t, "/path/to/cert", opt.Webhook.CertDir)
	assert.Equal(t, 8080, opt.Webhook.WebhookPort)

	// Verify Observability flags
	assert.Equal(t, "/metrics", opt.Observability.MetricsAddr)
	assert.Equal(t, true, opt.Observability.LogDebug)
	assert.Equal(t, "/path/to/log", opt.Observability.LogFilePath)
	assert.Equal(t, uint64(50), opt.Observability.LogFileMaxSize)

	// Verify Kubernetes flags
	assert.Equal(t, 3*time.Second, opt.Kubernetes.InformerSyncPeriod)
	assert.Equal(t, float64(200), opt.Kubernetes.QPS)
	assert.Equal(t, 500, opt.Kubernetes.Burst)

	// Verify MultiCluster flags
	assert.Equal(t, true, opt.MultiCluster.EnableClusterGateway)
	assert.Equal(t, true, opt.MultiCluster.EnableClusterMetrics)
	assert.Equal(t, 5*time.Second, opt.MultiCluster.ClusterMetricsInterval)

	// Verify CUE flags
	assert.True(t, opt.CUE.EnableExternalPackage)
	assert.True(t, opt.CUE.EnableExternalPackageWatch)

	// Verify Application flags
	assert.Equal(t, 5*time.Second, opt.Application.ReSyncPeriod)

	// Verify OAM flags
	assert.Equal(t, "custom-namespace", opt.OAM.SystemDefinitionNamespace)

	// Verify Performance flags
	assert.Equal(t, true, opt.Performance.PerfEnabled)

	// Verify Controller flags
	assert.Equal(t, 100, opt.Controller.RevisionLimit)
	assert.Equal(t, 20, opt.Controller.AppRevisionLimit)
	assert.Equal(t, 30, opt.Controller.DefRevisionLimit)
	assert.Equal(t, false, opt.Controller.AutoGenWorkloadDefinition)
	assert.Equal(t, 8, opt.Controller.ConcurrentReconciles)
	assert.Equal(t, true, opt.Controller.IgnoreAppWithoutControllerRequirement)
	assert.Equal(t, true, opt.Controller.IgnoreDefinitionWithoutControllerRequirement)

	// Verify Workflow flags
	assert.Equal(t, 30, opt.Workflow.MaxWaitBackoffTime)
	assert.Equal(t, 150, opt.Workflow.MaxFailedBackoffTime)
	assert.Equal(t, 5, opt.Workflow.MaxStepErrorRetryTimes)

	// Verify Resource flags
	assert.Equal(t, 5, opt.Resource.MaxDispatchConcurrent)
}

func TestCuexOptions_SyncToGlobals(t *testing.T) {
	// Reset globals
	cuex.EnableExternalPackageForDefaultCompiler = false
	cuex.EnableExternalPackageWatchForDefaultCompiler = false

	opts := NewCoreOptions()
	fss := opts.Flags()

	args := []string{
		"--enable-external-package-for-default-compiler=true",
		"--enable-external-package-watch-for-default-compiler=true",
	}

	err := fss.FlagSet("cue").Parse(args)
	require.NoError(t, err)

	// Before sync, globals should still be false
	assert.False(t, cuex.EnableExternalPackageForDefaultCompiler)
	assert.False(t, cuex.EnableExternalPackageWatchForDefaultCompiler)

	// After sync, globals should be updated
	opts.CUE.SyncToCUEGlobals()
	assert.True(t, cuex.EnableExternalPackageForDefaultCompiler)
	assert.True(t, cuex.EnableExternalPackageWatchForDefaultCompiler)
}

func TestWorkflowOptions_SyncToGlobals(t *testing.T) {
	// Store original values
	origWait := wfTypes.MaxWorkflowWaitBackoffTime
	origFailed := wfTypes.MaxWorkflowFailedBackoffTime
	origRetry := wfTypes.MaxWorkflowStepErrorRetryTimes

	// Restore after test
	defer func() {
		wfTypes.MaxWorkflowWaitBackoffTime = origWait
		wfTypes.MaxWorkflowFailedBackoffTime = origFailed
		wfTypes.MaxWorkflowStepErrorRetryTimes = origRetry
	}()

	opts := NewCoreOptions()
	fss := opts.Flags()

	args := []string{
		"--max-workflow-wait-backoff-time=120",
		"--max-workflow-failed-backoff-time=600",
		"--max-workflow-step-error-retry-times=20",
	}

	err := fss.FlagSet("workflow").Parse(args)
	require.NoError(t, err)

	// Verify struct fields are updated
	assert.Equal(t, 120, opts.Workflow.MaxWaitBackoffTime)
	assert.Equal(t, 600, opts.Workflow.MaxFailedBackoffTime)
	assert.Equal(t, 20, opts.Workflow.MaxStepErrorRetryTimes)

	// After sync, globals should be updated
	opts.Workflow.SyncToWorkflowGlobals()
	assert.Equal(t, 120, wfTypes.MaxWorkflowWaitBackoffTime)
	assert.Equal(t, 600, wfTypes.MaxWorkflowFailedBackoffTime)
	assert.Equal(t, 20, wfTypes.MaxWorkflowStepErrorRetryTimes)
}

func TestOAMOptions_SyncToGlobals(t *testing.T) {
	// Store original value
	origNamespace := oam.SystemDefinitionNamespace

	// Restore after test
	defer func() {
		oam.SystemDefinitionNamespace = origNamespace
	}()

	opts := NewCoreOptions()
	fss := opts.Flags()

	args := []string{
		"--system-definition-namespace=custom-system",
	}

	err := fss.FlagSet("oam").Parse(args)
	require.NoError(t, err)

	// Verify struct field is updated
	assert.Equal(t, "custom-system", opts.OAM.SystemDefinitionNamespace)

	// After sync, global should be updated
	opts.OAM.SyncToOAMGlobals()
	assert.Equal(t, "custom-system", oam.SystemDefinitionNamespace)
}

func TestPerformanceOptions_SyncToGlobals(t *testing.T) {
	// Store original value
	origPerf := commonconfig.PerfEnabled

	// Restore after test
	defer func() {
		commonconfig.PerfEnabled = origPerf
	}()

	opts := NewCoreOptions()
	fss := opts.Flags()

	args := []string{
		"--perf-enabled=true",
	}

	err := fss.FlagSet("performance").Parse(args)
	require.NoError(t, err)

	// Verify struct field is updated
	assert.Equal(t, true, opts.Performance.PerfEnabled)

	// After sync, global should be updated
	opts.Performance.SyncToPerformanceGlobals()
	assert.True(t, commonconfig.PerfEnabled)
}

func TestApplicationOptions_SyncToGlobals(t *testing.T) {
	// Store original value
	origPeriod := commonconfig.ApplicationReSyncPeriod

	// Restore after test
	defer func() {
		commonconfig.ApplicationReSyncPeriod = origPeriod
	}()

	opts := NewCoreOptions()
	fss := opts.Flags()

	args := []string{
		"--application-re-sync-period=10m",
	}

	err := fss.FlagSet("application").Parse(args)
	require.NoError(t, err)

	// Verify struct field is updated
	assert.Equal(t, 10*time.Minute, opts.Application.ReSyncPeriod)

	// After sync, global should be updated
	opts.Application.SyncToApplicationGlobals()
	assert.Equal(t, 10*time.Minute, commonconfig.ApplicationReSyncPeriod)
}

func TestResourceOptions_SyncToGlobals(t *testing.T) {
	// Store original value
	origDispatch := resourcekeeper.MaxDispatchConcurrent

	// Restore after test
	defer func() {
		resourcekeeper.MaxDispatchConcurrent = origDispatch
	}()

	opts := NewCoreOptions()
	fss := opts.Flags()

	args := []string{
		"--max-dispatch-concurrent=25",
	}

	err := fss.FlagSet("resource").Parse(args)
	require.NoError(t, err)

	// Verify struct field is updated
	assert.Equal(t, 25, opts.Resource.MaxDispatchConcurrent)

	// After sync, global should be updated
	opts.Resource.SyncToResourceGlobals()
	assert.Equal(t, 25, resourcekeeper.MaxDispatchConcurrent)
}

func TestCoreOptions_InvalidValues(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name: "invalid boolean value",
			args: []string{
				"--enable-leader-election=notabool",
			},
			expectError: true,
			errorMsg:    "invalid argument",
		},
		{
			name: "invalid duration value",
			args: []string{
				"--leader-election-lease-duration=notaduration",
			},
			expectError: true,
			errorMsg:    "invalid argument",
		},
		{
			name: "invalid int value",
			args: []string{
				"--webhook-port=notanint",
			},
			expectError: true,
			errorMsg:    "invalid argument",
		},
		{
			name: "invalid float value",
			args: []string{
				"--kube-api-qps=notafloat",
			},
			expectError: true,
			errorMsg:    "invalid argument",
		},
		{
			name: "invalid uint64 value",
			args: []string{
				"--log-file-max-size=-100",
			},
			expectError: true,
			errorMsg:    "invalid argument",
		},
		{
			name: "unknown flag",
			args: []string{
				"--unknown-flag=value",
			},
			expectError: true,
			errorMsg:    "unknown flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			opt := NewCoreOptions()

			for _, f := range opt.Flags().FlagSets {
				fs.AddFlagSet(f)
			}

			err := fs.Parse(tt.args)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCoreOptions_PartialConfiguration(t *testing.T) {
	// Test that partial configuration works correctly
	// and doesn't override other defaults
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opt := NewCoreOptions()

	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	// Only set a few flags
	args := []string{
		"--enable-leader-election=true",
		"--log-debug=true",
		"--perf-enabled=true",
	}

	err := fs.Parse(args)
	require.NoError(t, err)

	// Check that specified flags are updated
	assert.Equal(t, true, opt.Server.EnableLeaderElection)
	assert.Equal(t, true, opt.Observability.LogDebug)
	assert.Equal(t, true, opt.Performance.PerfEnabled)

	// Check that unspecified flags retain defaults
	assert.Equal(t, ":9440", opt.Server.HealthAddr)
	assert.Equal(t, "Local", opt.Server.StorageDriver)
	assert.Equal(t, false, opt.Webhook.UseWebhook)
	assert.Equal(t, ":8080", opt.Observability.MetricsAddr)
	assert.Equal(t, 10*time.Hour, opt.Kubernetes.InformerSyncPeriod)
	assert.Equal(t, 10, opt.Resource.MaxDispatchConcurrent)
}

func TestCoreOptions_FlagSetsOrganization(t *testing.T) {
	opt := NewCoreOptions()
	fss := opt.Flags()

	// Verify that all expected flag sets are created
	expectedFlagSets := []string{
		"server",
		"webhook",
		"observability",
		"kubernetes",
		"multicluster",
		"cue",
		"application",
		"oam",
		"performance",
		"admission",
		"resource",
		"workflow",
		"controller",
		"client",
		"reconcile",
		"sharding",
		"feature",
		"profiling",
		"klog",
	}

	for _, name := range expectedFlagSets {
		fs := fss.FlagSet(name)
		assert.NotNil(t, fs, "FlagSet %s should exist", name)
	}
}

func TestCoreOptions_FlagHelp(t *testing.T) {
	opt := NewCoreOptions()
	fss := opt.Flags()

	// Test that flags have proper help messages
	serverFS := fss.FlagSet("server")
	flag := serverFS.Lookup("enable-leader-election")
	assert.NotNil(t, flag)
	assert.Contains(t, flag.Usage, "Enable leader election")

	webhookFS := fss.FlagSet("webhook")
	flag = webhookFS.Lookup("use-webhook")
	assert.NotNil(t, flag)
	assert.Contains(t, flag.Usage, "Enable Admission Webhook")

	obsFS := fss.FlagSet("observability")
	flag = obsFS.Lookup("log-debug")
	assert.NotNil(t, flag)
	assert.Contains(t, flag.Usage, "Enable debug logs")
}

func TestCoreOptions_MultipleSyncCalls(t *testing.T) {
	// Store original values
	origCUEExternal := cuex.EnableExternalPackageForDefaultCompiler
	origCUEWatch := cuex.EnableExternalPackageWatchForDefaultCompiler
	origWait := wfTypes.MaxWorkflowWaitBackoffTime
	origDispatch := resourcekeeper.MaxDispatchConcurrent
	origOAMNamespace := oam.SystemDefinitionNamespace
	origAppPeriod := commonconfig.ApplicationReSyncPeriod
	origPerf := commonconfig.PerfEnabled

	// Restore after test
	defer func() {
		cuex.EnableExternalPackageForDefaultCompiler = origCUEExternal
		cuex.EnableExternalPackageWatchForDefaultCompiler = origCUEWatch
		wfTypes.MaxWorkflowWaitBackoffTime = origWait
		resourcekeeper.MaxDispatchConcurrent = origDispatch
		oam.SystemDefinitionNamespace = origOAMNamespace
		commonconfig.ApplicationReSyncPeriod = origAppPeriod
		commonconfig.PerfEnabled = origPerf
	}()

	// Test that calling sync multiple times doesn't cause issues
	opts := NewCoreOptions()

	// Set some values
	opts.CUE.EnableExternalPackage = true
	opts.CUE.EnableExternalPackageWatch = false
	opts.Workflow.MaxWaitBackoffTime = 100
	opts.Resource.MaxDispatchConcurrent = 20
	opts.OAM.SystemDefinitionNamespace = "test-system"
	opts.Application.ReSyncPeriod = 15 * time.Minute
	opts.Performance.PerfEnabled = true

	// Call sync multiple times
	opts.CUE.SyncToCUEGlobals()
	opts.CUE.SyncToCUEGlobals()

	opts.Workflow.SyncToWorkflowGlobals()
	opts.Workflow.SyncToWorkflowGlobals()

	opts.Resource.SyncToResourceGlobals()
	opts.Resource.SyncToResourceGlobals()

	opts.OAM.SyncToOAMGlobals()
	opts.OAM.SyncToOAMGlobals()

	opts.Application.SyncToApplicationGlobals()
	opts.Application.SyncToApplicationGlobals()

	opts.Performance.SyncToPerformanceGlobals()
	opts.Performance.SyncToPerformanceGlobals()

	// Verify values are still correct
	assert.True(t, cuex.EnableExternalPackageForDefaultCompiler)
	assert.False(t, cuex.EnableExternalPackageWatchForDefaultCompiler)
	assert.Equal(t, 100, wfTypes.MaxWorkflowWaitBackoffTime)
	assert.Equal(t, 20, resourcekeeper.MaxDispatchConcurrent)
	assert.Equal(t, "test-system", oam.SystemDefinitionNamespace)
	assert.Equal(t, 15*time.Minute, commonconfig.ApplicationReSyncPeriod)
	assert.True(t, commonconfig.PerfEnabled)
}

func TestCoreOptions_SpecialCharactersInStrings(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opt := NewCoreOptions()

	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	// Test with special characters and spaces in paths
	args := []string{
		`--webhook-cert-dir=/path/with spaces/and-special!@#$%chars`,
		`--log-file-path=/var/log/kubevela/日本語/логи.log`,
		`--health-addr=[::1]:8080`,
		`--metrics-addr=0.0.0.0:9090`,
	}

	err := fs.Parse(args)
	require.NoError(t, err)

	assert.Equal(t, `/path/with spaces/and-special!@#$%chars`, opt.Webhook.CertDir)
	assert.Equal(t, `/var/log/kubevela/日本語/логи.log`, opt.Observability.LogFilePath)
	assert.Equal(t, `[::1]:8080`, opt.Server.HealthAddr)
	assert.Equal(t, `0.0.0.0:9090`, opt.Observability.MetricsAddr)
}

func TestCoreOptions_ConcurrentAccess(t *testing.T) {
	// Test that the options can be accessed concurrently safely
	opt := NewCoreOptions()

	// Set some values
	opt.Server.EnableLeaderElection = true
	opt.Workflow.MaxWaitBackoffTime = 100
	opt.Resource.MaxDispatchConcurrent = 20

	// Simulate concurrent access
	done := make(chan bool, 3)

	go func() {
		for i := 0; i < 100; i++ {
			_ = opt.Server.EnableLeaderElection
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = opt.Workflow.MaxWaitBackoffTime
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = opt.Resource.MaxDispatchConcurrent
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestCoreOptions_NilPointerSafety(t *testing.T) {
	// Ensure NewCoreOptions never returns nil pointers
	opt := NewCoreOptions()

	// All config modules should be non-nil
	assert.NotNil(t, opt.Server)
	assert.NotNil(t, opt.Webhook)
	assert.NotNil(t, opt.Observability)
	assert.NotNil(t, opt.Kubernetes)
	assert.NotNil(t, opt.MultiCluster)
	assert.NotNil(t, opt.CUE)
	assert.NotNil(t, opt.Application)
	assert.NotNil(t, opt.OAM)
	assert.NotNil(t, opt.Performance)
	assert.NotNil(t, opt.Workflow)
	assert.NotNil(t, opt.Admission)
	assert.NotNil(t, opt.Resource)
	assert.NotNil(t, opt.Client)
	assert.NotNil(t, opt.Reconcile)
	assert.NotNil(t, opt.Sharding)
	assert.NotNil(t, opt.Feature)
	assert.NotNil(t, opt.Profiling)
	assert.NotNil(t, opt.KLog)
	assert.NotNil(t, opt.Controller)
}

func TestCoreOptions_FlagPrecedence(t *testing.T) {
	// Test that later flags override earlier ones
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	opt := NewCoreOptions()

	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	// Parse with one value, then parse again with different value
	args1 := []string{"--webhook-port=8080"}
	err := fs.Parse(args1)
	require.NoError(t, err)
	assert.Equal(t, 8080, opt.Webhook.WebhookPort)

	// Reset and parse with different value
	fs = pflag.NewFlagSet("test", pflag.ContinueOnError)
	opt = NewCoreOptions()
	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	args2 := []string{"--webhook-port=9090"}
	err = fs.Parse(args2)
	require.NoError(t, err)
	assert.Equal(t, 9090, opt.Webhook.WebhookPort)
}

func TestCoreOptions_AllConfigModulesHaveFlags(t *testing.T) {
	// Ensure every config module registers at least one flag
	opt := NewCoreOptions()
	fss := opt.Flags()

	configsWithExpectedFlags := map[string][]string{
		"server":        {"health-addr", "storage-driver", "enable-leader-election"},
		"webhook":       {"use-webhook", "webhook-cert-dir", "webhook-port"},
		"observability": {"metrics-addr", "log-debug", "log-file-path"},
		"kubernetes":    {"informer-sync-period", "kube-api-qps", "kube-api-burst"},
		"multicluster":  {"enable-cluster-gateway", "enable-cluster-metrics"},
		"cue":           {"enable-external-package-for-default-compiler"},
		"application":   {"application-re-sync-period"},
		"oam":           {"system-definition-namespace"},
		"controller":    {"revision-limit", "application-revision-limit", "definition-revision-limit"},
		"performance":   {"perf-enabled"},
		"workflow":      {"max-workflow-wait-backoff-time"},
		"resource":      {"max-dispatch-concurrent"},
	}

	for setName, expectedFlags := range configsWithExpectedFlags {
		fs := fss.FlagSet(setName)
		assert.NotNil(t, fs, "FlagSet %s should exist", setName)

		for _, flagName := range expectedFlags {
			flag := fs.Lookup(flagName)
			assert.NotNil(t, flag, "Flag %s should exist in flagset %s", flagName, setName)
		}
	}
}

func TestCoreOptions_CLIOverridesWork(t *testing.T) {
	// This test verifies that CLI flags correctly override default values
	// and that the sync methods properly propagate these values to globals

	// Store original globals to restore after test
	origWait := wfTypes.MaxWorkflowWaitBackoffTime
	origFailed := wfTypes.MaxWorkflowFailedBackoffTime
	origRetry := wfTypes.MaxWorkflowStepErrorRetryTimes
	origDispatch := resourcekeeper.MaxDispatchConcurrent
	origOAMNamespace := oam.SystemDefinitionNamespace
	origAppPeriod := commonconfig.ApplicationReSyncPeriod
	origPerf := commonconfig.PerfEnabled
	origCUEExternal := cuex.EnableExternalPackageForDefaultCompiler
	origCUEWatch := cuex.EnableExternalPackageWatchForDefaultCompiler

	defer func() {
		wfTypes.MaxWorkflowWaitBackoffTime = origWait
		wfTypes.MaxWorkflowFailedBackoffTime = origFailed
		wfTypes.MaxWorkflowStepErrorRetryTimes = origRetry
		resourcekeeper.MaxDispatchConcurrent = origDispatch
		oam.SystemDefinitionNamespace = origOAMNamespace
		commonconfig.ApplicationReSyncPeriod = origAppPeriod
		commonconfig.PerfEnabled = origPerf
		cuex.EnableExternalPackageForDefaultCompiler = origCUEExternal
		cuex.EnableExternalPackageWatchForDefaultCompiler = origCUEWatch
	}()

	opt := NewCoreOptions()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	// Verify defaults first
	assert.Equal(t, 60, opt.Workflow.MaxWaitBackoffTime, "Default should be 60")
	assert.Equal(t, 300, opt.Workflow.MaxFailedBackoffTime, "Default should be 300")
	assert.Equal(t, 10, opt.Workflow.MaxStepErrorRetryTimes, "Default should be 10")
	assert.Equal(t, 10, opt.Resource.MaxDispatchConcurrent, "Default should be 10")
	assert.Equal(t, "vela-system", opt.OAM.SystemDefinitionNamespace, "Default should be vela-system")
	assert.Equal(t, false, opt.Performance.PerfEnabled, "Default should be false")

	// Parse CLI args with overrides
	args := []string{
		"--max-workflow-wait-backoff-time=999",
		"--max-workflow-failed-backoff-time=888",
		"--max-workflow-step-error-retry-times=77",
		"--max-dispatch-concurrent=66",
		"--system-definition-namespace=custom-ns",
		"--application-re-sync-period=20m",
		"--perf-enabled=true",
		"--enable-external-package-for-default-compiler=true",
		"--enable-external-package-watch-for-default-compiler=true",
	}

	err := fs.Parse(args)
	require.NoError(t, err)

	// Verify struct fields got CLI values (not defaults)
	assert.Equal(t, 999, opt.Workflow.MaxWaitBackoffTime, "CLI override should be 999")
	assert.Equal(t, 888, opt.Workflow.MaxFailedBackoffTime, "CLI override should be 888")
	assert.Equal(t, 77, opt.Workflow.MaxStepErrorRetryTimes, "CLI override should be 77")
	assert.Equal(t, 66, opt.Resource.MaxDispatchConcurrent, "CLI override should be 66")
	assert.Equal(t, "custom-ns", opt.OAM.SystemDefinitionNamespace, "CLI override should be custom-ns")
	assert.Equal(t, 20*time.Minute, opt.Application.ReSyncPeriod, "CLI override should be 20m")
	assert.Equal(t, true, opt.Performance.PerfEnabled, "CLI override should be true")
	assert.Equal(t, true, opt.CUE.EnableExternalPackage, "CLI override should be true")
	assert.Equal(t, true, opt.CUE.EnableExternalPackageWatch, "CLI override should be true")

	// Now sync to globals
	opt.Workflow.SyncToWorkflowGlobals()
	opt.Resource.SyncToResourceGlobals()
	opt.OAM.SyncToOAMGlobals()
	opt.Application.SyncToApplicationGlobals()
	opt.Performance.SyncToPerformanceGlobals()
	opt.CUE.SyncToCUEGlobals()

	// Verify globals got the CLI values
	assert.Equal(t, 999, wfTypes.MaxWorkflowWaitBackoffTime, "Global should have CLI value")
	assert.Equal(t, 888, wfTypes.MaxWorkflowFailedBackoffTime, "Global should have CLI value")
	assert.Equal(t, 77, wfTypes.MaxWorkflowStepErrorRetryTimes, "Global should have CLI value")
	assert.Equal(t, 66, resourcekeeper.MaxDispatchConcurrent, "Global should have CLI value")
	assert.Equal(t, "custom-ns", oam.SystemDefinitionNamespace, "Global should have CLI value")
	assert.Equal(t, 20*time.Minute, commonconfig.ApplicationReSyncPeriod, "Global should have CLI value")
	assert.Equal(t, true, commonconfig.PerfEnabled, "Global should have CLI value")
	assert.Equal(t, true, cuex.EnableExternalPackageForDefaultCompiler, "Global should have CLI value")
	assert.Equal(t, true, cuex.EnableExternalPackageWatchForDefaultCompiler, "Global should have CLI value")
}

func TestCoreOptions_CompleteIntegration(t *testing.T) {
	// A comprehensive integration test
	opt := NewCoreOptions()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	for _, f := range opt.Flags().FlagSets {
		fs.AddFlagSet(f)
	}

	// Simulate a real-world configuration
	args := []string{
		// Production-like settings
		"--enable-leader-election=true",
		"--leader-election-namespace=vela-system",
		"--use-webhook=true",
		"--webhook-port=9443",
		"--metrics-addr=:8080",
		"--health-addr=:9440",
		"--log-debug=false",
		"--log-file-path=/var/log/vela/core.log",
		"--log-file-max-size=100",
		"--kube-api-qps=100",
		"--kube-api-burst=200",
		"--enable-cluster-gateway=true",
		"--enable-cluster-metrics=true",
		"--cluster-metrics-interval=30s",
		"--application-re-sync-period=10m",
		"--perf-enabled=true",
		"--max-dispatch-concurrent=20",
		"--max-workflow-wait-backoff-time=120",
		"--max-workflow-failed-backoff-time=600",
	}

	err := fs.Parse(args)
	require.NoError(t, err)

	// Verify the configuration is production-ready
	assert.True(t, opt.Server.EnableLeaderElection, "Leader election should be enabled in production")
	assert.Equal(t, "vela-system", opt.Server.LeaderElectionNamespace)
	assert.True(t, opt.Webhook.UseWebhook, "Webhook should be enabled in production")
	assert.Equal(t, 9443, opt.Webhook.WebhookPort)
	assert.False(t, opt.Observability.LogDebug, "Debug logging should be disabled in production")
	assert.NotEmpty(t, opt.Observability.LogFilePath, "Log file path should be set in production")

	// Verify performance settings
	assert.True(t, opt.Performance.PerfEnabled)
	assert.Equal(t, 20, opt.Resource.MaxDispatchConcurrent)

	// Verify cluster settings
	assert.True(t, opt.MultiCluster.EnableClusterGateway)
	assert.True(t, opt.MultiCluster.EnableClusterMetrics)
	assert.Equal(t, 30*time.Second, opt.MultiCluster.ClusterMetricsInterval)

	// Sync all configurations that need it
	opt.CUE.SyncToCUEGlobals()
	opt.Workflow.SyncToWorkflowGlobals()
	opt.Resource.SyncToResourceGlobals()
	opt.OAM.SyncToOAMGlobals()
	opt.Application.SyncToApplicationGlobals()
	opt.Performance.SyncToPerformanceGlobals()

	// Verify sync worked
	assert.Equal(t, 20, resourcekeeper.MaxDispatchConcurrent)
	assert.Equal(t, 120, wfTypes.MaxWorkflowWaitBackoffTime)
	assert.Equal(t, 600, wfTypes.MaxWorkflowFailedBackoffTime)
	assert.Equal(t, "vela-system", oam.SystemDefinitionNamespace)
	assert.Equal(t, 10*time.Minute, commonconfig.ApplicationReSyncPeriod)
	assert.True(t, commonconfig.PerfEnabled)
}
