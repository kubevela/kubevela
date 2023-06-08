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
	"github.com/spf13/pflag"

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
		"--leader-election-resource-lock=/leases",
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
		UseWebhook:                 true,
		CertDir:                    "/path/to/cert",
		WebhookPort:                8080,
		MetricsAddr:                "/metrics",
		EnableLeaderElection:       true,
		LeaderElectionNamespace:    "test-namespace",
		LogFilePath:                "/path/to/log",
		LogFileMaxSize:             50,
		LogDebug:                   true,
		ControllerArgs:             &oamcontroller.Args{},
		HealthAddr:                 "/healthz",
		StorageDriver:              "",
		InformerSyncPeriod:         3 * time.Second,
		QPS:                        200,
		Burst:                      500,
		LeaderElectionResourceLock: "/leases",
		LeaseDuration:              3 * time.Second,
		RenewDeadLine:              5 * time.Second,
		RetryPeriod:                3 * time.Second,
		EnableClusterGateway:       true,
		EnableClusterMetrics:       true,
		ClusterMetricsInterval:     5 * time.Second,
	}

	if !cmp.Equal(opt, expected, cmp.AllowUnexported(CoreOptions{})) {
		t.Errorf("Flags() diff: %v", cmp.Diff(opt, expected, cmp.AllowUnexported(CoreOptions{})))
	}
}
