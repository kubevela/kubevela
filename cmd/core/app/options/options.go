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
	cliflag "k8s.io/component-base/cli/flag"

	"github.com/oam-dev/kubevela/cmd/core/app/config"
)

// CoreOptions contains everything necessary to create and run vela-core
type CoreOptions struct {
	// Config modules - clean, well-organized configuration
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
	Controller    *config.ControllerConfig
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
	controller := config.NewControllerConfig()

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
		Controller:    controller,
	}

	return s
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
	s.Controller.AddFlags(fss.FlagSet("controller"))

	// External package configurations (now wrapped in config modules)
	s.Client.AddFlags(fss.FlagSet("client"))
	s.Reconcile.AddFlags(fss.FlagSet("reconcile"))
	s.Sharding.AddFlags(fss.FlagSet("sharding"))
	s.Feature.AddFlags(fss.FlagSet("feature"))
	s.Profiling.AddFlags(fss.FlagSet("profiling"))
	s.KLog.AddFlags(fss.FlagSet("klog"))

	return fss
}
