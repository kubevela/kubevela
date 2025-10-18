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
	"time"

	cliflag "k8s.io/component-base/cli/flag"

	"github.com/oam-dev/kubevela/cmd/core/app/config"
	oamcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

// CoreOptions contains everything necessary to create and run vela-core
type CoreOptions struct {
	// Embedded config modules
	Server        *config.ServerConfig
	Webhook       *config.WebhookConfig
	Observability *config.ObservabilityConfig
	Kubernetes    *config.KubernetesConfig
	MultiCluster  *config.MultiClusterConfig
	CUE           *config.CUEConfig
	Application   *config.ApplicationConfig
	OAM           *config.OAMConfig
	Performance   *config.PerformanceConfig
	Workflow      *config.WorkflowConfig
	Admission     *config.AdmissionConfig
	Resource      *config.ResourceConfig
	Client        *config.ClientConfig
	Reconcile     *config.ReconcileConfig
	Sharding      *config.ShardingConfig
	Feature       *config.FeatureConfig
	Profiling     *config.ProfilingConfig
	KLog          *config.KLogConfig

	// Legacy fields maintained for backward compatibility
	UseWebhook              bool
	CertDir                 string
	WebhookPort             int
	MetricsAddr             string
	EnableLeaderElection    bool
	LeaderElectionNamespace string
	LogFilePath             string
	LogFileMaxSize          uint64
	LogDebug                bool
	DevLogs                 bool
	ControllerArgs          *oamcontroller.Args
	HealthAddr              string
	StorageDriver           string
	InformerSyncPeriod      time.Duration
	QPS                     float64
	Burst                   int
	LeaseDuration           time.Duration
	RenewDeadLine           time.Duration
	RetryPeriod             time.Duration
	EnableClusterGateway    bool
	EnableClusterMetrics    bool
	ClusterMetricsInterval  time.Duration
}

// NewCoreOptions creates a new NewVelaCoreOptions object with default parameters
func NewCoreOptions() *CoreOptions {
	// Initialize config modules
	server := config.NewServerConfig()
	webhook := config.NewWebhookConfig()
	observability := config.NewObservabilityConfig()
	kubernetes := config.NewKubernetesConfig()
	multiCluster := config.NewMultiClusterConfig()
	cue := config.NewCUEConfig()
	application := config.NewApplicationConfig()
	oam := config.NewOAMConfig()
	performance := config.NewPerformanceConfig()
	workflow := config.NewWorkflowConfig()
	admission := config.NewAdmissionConfig()
	resource := config.NewResourceConfig()
	client := config.NewClientConfig()
	reconcile := config.NewReconcileConfig()
	sharding := config.NewShardingConfig()
	feature := config.NewFeatureConfig()
	profiling := config.NewProfilingConfig()
	klog := config.NewKLogConfig(observability)

	s := &CoreOptions{
		// Config modules
		Server:        server,
		Webhook:       webhook,
		Observability: observability,
		Kubernetes:    kubernetes,
		MultiCluster:  multiCluster,
		CUE:           cue,
		Application:   application,
		OAM:           oam,
		Performance:   performance,
		Workflow:      workflow,
		Admission:     admission,
		Resource:      resource,
		Client:        client,
		Reconcile:     reconcile,
		Sharding:      sharding,
		Feature:       feature,
		Profiling:     profiling,
		KLog:          klog,

		// Initialize legacy fields from config modules for backward compatibility
		UseWebhook:              webhook.UseWebhook,
		CertDir:                 webhook.CertDir,
		WebhookPort:             webhook.WebhookPort,
		MetricsAddr:             observability.MetricsAddr,
		EnableLeaderElection:    server.EnableLeaderElection,
		LeaderElectionNamespace: server.LeaderElectionNamespace,
		LogFilePath:             observability.LogFilePath,
		LogFileMaxSize:          observability.LogFileMaxSize,
		LogDebug:                observability.LogDebug,
		DevLogs:                 observability.DevLogs,
		HealthAddr:              server.HealthAddr,
		StorageDriver:           server.StorageDriver,
		InformerSyncPeriod:      kubernetes.InformerSyncPeriod,
		QPS:                     kubernetes.QPS,
		Burst:                   kubernetes.Burst,
		LeaseDuration:           server.LeaseDuration,
		RenewDeadLine:           server.RenewDeadline,
		RetryPeriod:             server.RetryPeriod,
		EnableClusterGateway:    multiCluster.EnableClusterGateway,
		EnableClusterMetrics:    multiCluster.EnableClusterMetrics,
		ClusterMetricsInterval:  multiCluster.ClusterMetricsInterval,

		// Controller args remain as is
		ControllerArgs: &oamcontroller.Args{
			RevisionLimit:                                50,
			AppRevisionLimit:                             10,
			DefRevisionLimit:                             20,
			AutoGenWorkloadDefinition:                    true,
			ConcurrentReconciles:                         4,
			IgnoreAppWithoutControllerRequirement:        false,
			IgnoreDefinitionWithoutControllerRequirement: false,
		},
	}

	return s
}

// SyncLegacyFields syncs values from config modules back to legacy fields
// This must be called after flag parsing to ensure legacy code sees the updated values
func (s *CoreOptions) SyncLegacyFields() {
	// Sync Server config
	s.HealthAddr = s.Server.HealthAddr
	s.StorageDriver = s.Server.StorageDriver
	s.EnableLeaderElection = s.Server.EnableLeaderElection
	s.LeaderElectionNamespace = s.Server.LeaderElectionNamespace
	s.LeaseDuration = s.Server.LeaseDuration
	s.RenewDeadLine = s.Server.RenewDeadline
	s.RetryPeriod = s.Server.RetryPeriod

	// Sync Webhook config
	s.UseWebhook = s.Webhook.UseWebhook
	s.CertDir = s.Webhook.CertDir
	s.WebhookPort = s.Webhook.WebhookPort

	// Sync Observability config
	s.MetricsAddr = s.Observability.MetricsAddr
	s.LogFilePath = s.Observability.LogFilePath
	s.LogFileMaxSize = s.Observability.LogFileMaxSize
	s.LogDebug = s.Observability.LogDebug
	s.DevLogs = s.Observability.DevLogs

	// Sync Kubernetes config
	s.QPS = s.Kubernetes.QPS
	s.Burst = s.Kubernetes.Burst
	s.InformerSyncPeriod = s.Kubernetes.InformerSyncPeriod

	// Sync MultiCluster config
	s.EnableClusterGateway = s.MultiCluster.EnableClusterGateway
	s.EnableClusterMetrics = s.MultiCluster.EnableClusterMetrics
	s.ClusterMetricsInterval = s.MultiCluster.ClusterMetricsInterval
}

// Flags returns the complete NamedFlagSets
func (s *CoreOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}

	// Use config modules to register flags - clean delegation pattern
	s.Server.AddFlags(fss.FlagSet("server"))
	s.Webhook.AddFlags(fss.FlagSet("webhook"))
	s.Observability.AddFlags(fss.FlagSet("observability"))
	s.Kubernetes.AddFlags(fss.FlagSet("kubernetes"))
	s.MultiCluster.AddFlags(fss.FlagSet("multicluster"))
	s.CUE.AddFlags(fss.FlagSet("cue"))
	s.Application.AddFlags(fss.FlagSet("application"))
	s.OAM.AddFlags(fss.FlagSet("oam"))
	s.Performance.AddFlags(fss.FlagSet("performance"))
	s.Admission.AddFlags(fss.FlagSet("admission"))
	s.Resource.AddFlags(fss.FlagSet("resource"))
	s.Workflow.AddFlags(fss.FlagSet("workflow"))

	// Controller Arguments
	s.ControllerArgs.AddFlags(fss.FlagSet("controller"), s.ControllerArgs)

	// External package configurations (now wrapped in config modules)
	s.Client.AddFlags(fss.FlagSet("client"))
	s.Reconcile.AddFlags(fss.FlagSet("reconcile"))
	s.Sharding.AddFlags(fss.FlagSet("sharding"))
	s.Feature.AddFlags(fss.FlagSet("feature"))
	s.Profiling.AddFlags(fss.FlagSet("profiling"))
	s.KLog.AddFlags(fss.FlagSet("klog"))

	return fss
}
