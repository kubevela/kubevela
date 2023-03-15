/*
Copyright 2022 The KubeVela Authors.

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
	"strconv"
	"time"

	pkgclient "github.com/kubevela/pkg/controller/client"
	ctrlrec "github.com/kubevela/pkg/controller/reconciler"
	"github.com/kubevela/pkg/controller/sharding"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	utillog "github.com/kubevela/pkg/util/log"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	cliflag "k8s.io/component-base/cli/flag"

	standardcontroller "github.com/oam-dev/kubevela/pkg/controller"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	oamcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// CoreOptions contains everything necessary to create and run vela-core
type CoreOptions struct {
	UseWebhook                 bool
	CertDir                    string
	WebhookPort                int
	MetricsAddr                string
	EnableLeaderElection       bool
	LeaderElectionNamespace    string
	LogFilePath                string
	LogFileMaxSize             uint64
	LogDebug                   bool
	ControllerArgs             *oamcontroller.Args
	HealthAddr                 string
	DisableCaps                string
	StorageDriver              string
	InformerSyncPeriod         time.Duration
	QPS                        float64
	Burst                      int
	PprofAddr                  string
	LeaderElectionResourceLock string
	LeaseDuration              time.Duration
	RenewDeadLine              time.Duration
	RetryPeriod                time.Duration
	EnableClusterGateway       bool
	EnableClusterMetrics       bool
	ClusterMetricsInterval     time.Duration
}

// NewCoreOptions creates a new NewVelaCoreOptions object with default parameters
func NewCoreOptions() *CoreOptions {
	s := &CoreOptions{
		UseWebhook:              false,
		CertDir:                 "/k8s-webhook-server/serving-certs",
		WebhookPort:             9443,
		MetricsAddr:             ":8080",
		EnableLeaderElection:    false,
		LeaderElectionNamespace: "",
		LogFilePath:             "",
		LogFileMaxSize:          1024,
		LogDebug:                false,
		ControllerArgs: &oamcontroller.Args{
			RevisionLimit:                                50,
			AppRevisionLimit:                             10,
			DefRevisionLimit:                             20,
			CustomRevisionHookURL:                        "",
			AutoGenWorkloadDefinition:                    true,
			ConcurrentReconciles:                         4,
			DependCheckWait:                              30 * time.Second,
			OAMSpecVer:                                   "v0.3",
			EnableCompatibility:                          false,
			IgnoreAppWithoutControllerRequirement:        false,
			IgnoreDefinitionWithoutControllerRequirement: false,
		},
		HealthAddr:                 ":9440",
		DisableCaps:                "",
		StorageDriver:              "Local",
		InformerSyncPeriod:         10 * time.Hour,
		QPS:                        50,
		Burst:                      100,
		PprofAddr:                  "",
		LeaderElectionResourceLock: "configmapsleases",
		LeaseDuration:              15 * time.Second,
		RenewDeadLine:              10 * time.Second,
		RetryPeriod:                2 * time.Second,
		EnableClusterGateway:       false,
		EnableClusterMetrics:       false,
		ClusterMetricsInterval:     15 * time.Second,
	}
	return s
}

// Flags returns the complete NamedFlagSets
func (s *CoreOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}

	gfs := fss.FlagSet("generic")
	gfs.BoolVar(&s.UseWebhook, "use-webhook", s.UseWebhook, "Enable Admission Webhook")
	gfs.StringVar(&s.CertDir, "webhook-cert-dir", s.CertDir, "Admission webhook cert/key dir.")
	gfs.IntVar(&s.WebhookPort, "webhook-port", s.WebhookPort, "admission webhook listen address")
	gfs.StringVar(&s.MetricsAddr, "metrics-addr", s.MetricsAddr, "The address the metric endpoint binds to.")
	gfs.BoolVar(&s.EnableLeaderElection, "enable-leader-election", s.EnableLeaderElection,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	gfs.StringVar(&s.LeaderElectionNamespace, "leader-election-namespace", s.LeaderElectionNamespace,
		"Determines the namespace in which the leader election configmap will be created.")
	gfs.StringVar(&s.LogFilePath, "log-file-path", s.LogFilePath, "The file to write logs to.")
	gfs.Uint64Var(&s.LogFileMaxSize, "log-file-max-size", s.LogFileMaxSize, "Defines the maximum size a log file can grow to, Unit is megabytes.")
	gfs.BoolVar(&s.LogDebug, "log-debug", s.LogDebug, "Enable debug logs for development purpose")
	gfs.StringVar(&s.HealthAddr, "health-addr", s.HealthAddr, "The address the health endpoint binds to.")
	gfs.StringVar(&s.DisableCaps, "disable-caps", s.DisableCaps, "To be disabled builtin capability list.")
	gfs.DurationVar(&s.InformerSyncPeriod, "informer-sync-period", s.InformerSyncPeriod,
		"The re-sync period for informer in controller-runtime. This is a system-level configuration.")
	gfs.Float64Var(&s.QPS, "kube-api-qps", s.QPS, "the qps for reconcile clients. Low qps may lead to low throughput. High qps may give stress to api-server. Raise this value if concurrent-reconciles is set to be high.")
	gfs.IntVar(&s.Burst, "kube-api-burst", s.Burst, "the burst for reconcile clients. Recommend setting it qps*2.")
	gfs.StringVar(&s.PprofAddr, "pprof-addr", s.PprofAddr, "The address for pprof to use while exporting profiling results. The default value is empty which means do not expose it. Set it to address like :6666 to expose it.")
	gfs.StringVar(&s.LeaderElectionResourceLock, "leader-election-resource-lock", s.LeaderElectionResourceLock, "The resource lock to use for leader election")
	gfs.DurationVar(&s.LeaseDuration, "leader-election-lease-duration", s.LeaseDuration,
		"The duration that non-leader candidates will wait to force acquire leadership")
	gfs.DurationVar(&s.RenewDeadLine, "leader-election-renew-deadline", s.RenewDeadLine,
		"The duration that the acting controlplane will retry refreshing leadership before giving up")
	gfs.DurationVar(&s.RetryPeriod, "leader-election-retry-period", s.RetryPeriod,
		"The duration the LeaderElector clients should wait between tries of actions")
	gfs.BoolVar(&s.EnableClusterGateway, "enable-cluster-gateway", s.EnableClusterGateway, "Enable cluster-gateway to use multicluster, disabled by default.")
	gfs.BoolVar(&s.EnableClusterMetrics, "enable-cluster-metrics", s.EnableClusterMetrics, "Enable cluster-metrics-management to collect metrics from clusters with cluster-gateway, disabled by default. When this param is enabled, enable-cluster-gateway should be enabled")
	gfs.DurationVar(&s.ClusterMetricsInterval, "cluster-metrics-interval", s.ClusterMetricsInterval, "The interval that ClusterMetricsMgr will collect metrics from clusters, default value is 15 seconds.")

	s.ControllerArgs.AddFlags(fss.FlagSet("controllerArgs"), s.ControllerArgs)

	cfs := fss.FlagSet("commonconfig")
	cfs.DurationVar(&commonconfig.ApplicationReSyncPeriod, "application-re-sync-period", commonconfig.ApplicationReSyncPeriod,
		"Re-sync period for application to re-sync, also known as the state-keep interval.")
	cfs.BoolVar(&commonconfig.PerfEnabled, "perf-enabled", commonconfig.PerfEnabled, "Enable performance logging for controllers, disabled by default.")

	ofs := fss.FlagSet("oam")
	ofs.StringVar(&oam.SystemDefinitionNamespace, "system-definition-namespace", "vela-system", "define the namespace of the system-level definition")

	standardcontroller.AddOptimizeFlags(fss.FlagSet("optimize"))
	standardcontroller.AddAdmissionFlags(fss.FlagSet("admission"))

	rfs := fss.FlagSet("resourcekeeper")
	rfs.IntVar(&resourcekeeper.MaxDispatchConcurrent, "max-dispatch-concurrent", 10, "Set the max dispatch concurrent number, default is 10")

	wfs := fss.FlagSet("wfTypes")
	wfs.IntVar(&wfTypes.MaxWorkflowWaitBackoffTime, "max-workflow-wait-backoff-time", 60, "Set the max workflow wait backoff time, default is 60")
	wfs.IntVar(&wfTypes.MaxWorkflowFailedBackoffTime, "max-workflow-failed-backoff-time", 300, "Set the max workflow wait backoff time, default is 300")
	wfs.IntVar(&wfTypes.MaxWorkflowStepErrorRetryTimes, "max-workflow-step-error-retry-times", 10, "Set the max workflow step error retry times, default is 10")

	pkgmulticluster.AddFlags(fss.FlagSet("multicluster"))
	ctrlrec.AddFlags(fss.FlagSet("controllerreconciles"))
	utilfeature.DefaultMutableFeatureGate.AddFlag(fss.FlagSet("featuregate"))
	sharding.AddFlags(fss.FlagSet("sharding"))
	kfs := fss.FlagSet("klog")
	pkgclient.AddTimeoutControllerClientFlags(fss.FlagSet("controllerclient"))
	utillog.AddFlags(kfs)

	if s.LogDebug {
		_ = kfs.Set("v", strconv.Itoa(int(commonconfig.LogDebug)))
	}

	if s.LogFilePath != "" {
		_ = kfs.Set("logtostderr", "false")
		_ = kfs.Set("log_file", s.LogFilePath)
		_ = kfs.Set("log_file_max_size", strconv.FormatUint(s.LogFileMaxSize, 10))
	}

	return fss
}
