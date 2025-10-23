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

package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	velaclient "github.com/kubevela/pkg/controller/client"
	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/meta"
	"github.com/kubevela/pkg/util/profiling"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/cmd/core/app/config"
	"github.com/oam-dev/kubevela/cmd/core/app/hooks"
	"github.com/oam-dev/kubevela/cmd/core/app/options"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cache"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	oamv1beta1 "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/watcher"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	oamwebhook "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev"
	"github.com/oam-dev/kubevela/version"
)

var (
	scheme             = common.Scheme
	waitSecretTimeout  = 90 * time.Second
	waitSecretInterval = 2 * time.Second
)

// NewCoreCommand creates a *cobra.Command object with default parameters
func NewCoreCommand() *cobra.Command {
	s := options.NewCoreOptions()
	cmd := &cobra.Command{
		Use:  "vela-core",
		Long: `The KubeVela controller manager is a daemon that embeds the core control loops shipped with KubeVela`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(signals.SetupSignalHandler(), s)
		},
		SilenceUsage: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			// Allow unknown flags for backward-compatibility.
			UnknownFlags: true,
		},
	}

	fs := cmd.Flags()
	namedFlagSets := s.Flags()
	for _, set := range namedFlagSets.FlagSets {
		fs.AddFlagSet(set)
	}
	meta.Name = types.VelaCoreName

	klog.InfoS("KubeVela information", "version", version.VelaVersion, "revision", version.GitRevision)
	klog.InfoS("Vela-Core init", "definition namespace", oam.SystemDefinitionNamespace)

	return cmd
}

func run(ctx context.Context, s *options.CoreOptions) error {
	// Sync configurations
	syncConfigurations(s)

	// Setup logging
	setupLogging(s.Observability)

	// Configure Kubernetes client
	restConfig, err := configureKubernetesClient(s.Kubernetes)
	if err != nil {
		return fmt.Errorf("failed to configure Kubernetes client: %w", err)
	}

	// Start profiling server
	go profiling.StartProfilingServer(nil)

	// Setup multi-cluster if enabled
	if s.MultiCluster.EnableClusterGateway {
		if err := setupMultiCluster(ctx, restConfig, s.MultiCluster); err != nil {
			return fmt.Errorf("failed to setup multi-cluster: %w", err)
		}
	}

	// Configure feature gates
	configureFeatureGates(s)

	// Create controller manager
	mgr, err := createControllerManager(ctx, restConfig, s)
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	// Register health checks
	if err := registerHealthChecks(mgr); err != nil {
		return fmt.Errorf("failed to register health checks: %w", err)
	}

	// Setup controllers based on sharding mode
	if err := setupControllers(ctx, mgr, s); err != nil {
		return fmt.Errorf("failed to setup controllers: %w", err)
	}

	// Start application monitor
	if err := startApplicationMonitor(ctx, mgr); err != nil {
		return fmt.Errorf("failed to start application monitor: %w", err)
	}

	// Start the manager
	if err := mgr.Start(ctx); err != nil {
		klog.ErrorS(err, "Failed to run manager")
		return err
	}

	// Cleanup
	performCleanup(s)
	klog.Info("Safely stops Program...")
	return nil
}

// syncConfigurations syncs parsed config values to external package global variables
func syncConfigurations(s *options.CoreOptions) {
	if s.Workflow != nil {
		s.Workflow.SyncToWorkflowGlobals()
	}
	if s.CUE != nil {
		s.CUE.SyncToCUEGlobals()
	}
	if s.Application != nil {
		s.Application.SyncToApplicationGlobals()
	}
	if s.Performance != nil {
		s.Performance.SyncToPerformanceGlobals()
	}
	if s.Resource != nil {
		s.Resource.SyncToResourceGlobals()
	}
	if s.OAM != nil {
		s.OAM.SyncToOAMGlobals()
	}
}

// setupLogging configures klog based on parsed observability settings
func setupLogging(obs *config.ObservabilityConfig) {
	// Configure klog verbosity
	if obs.LogDebug {
		_ = flag.Set("v", strconv.Itoa(int(commonconfig.LogDebug)))
	}

	// Configure log file output
	if obs.LogFilePath != "" {
		_ = flag.Set("logtostderr", "false")
		_ = flag.Set("log_file", obs.LogFilePath)
		_ = flag.Set("log_file_max_size", strconv.FormatUint(obs.LogFileMaxSize, 10))
	}

	// Set logger (use --dev-logs=true for local development)
	if obs.DevLogs {
		logOutput := newColorWriter(os.Stdout)
		klog.LogToStderr(false)
		klog.SetOutput(logOutput)
		ctrl.SetLogger(textlogger.NewLogger(textlogger.NewConfig(textlogger.Output(logOutput))))
	} else {
		ctrl.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))
	}
}

// configureKubernetesClient creates and configures the Kubernetes REST config
func configureKubernetesClient(k8sOpts *config.KubernetesConfig) (*rest.Config, error) {
	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = types.KubeVelaName + "/" + version.GitRevision
	restConfig.QPS = float32(k8sOpts.QPS)
	restConfig.Burst = k8sOpts.Burst
	restConfig.Wrap(auth.NewImpersonatingRoundTripper)

	klog.InfoS("Kubernetes Config Loaded",
		"UserAgent", restConfig.UserAgent,
		"QPS", restConfig.QPS,
		"Burst", restConfig.Burst,
	)

	return restConfig, nil
}

// setupMultiCluster initializes multi-cluster capability
func setupMultiCluster(ctx context.Context, restConfig *rest.Config, mcOpts *config.MultiClusterConfig) error {
	client, err := multicluster.Initialize(restConfig, true)
	if err != nil {
		klog.ErrorS(err, "failed to enable multi-cluster capability")
		return err
	}

	if mcOpts.EnableClusterMetrics {
		_, err := multicluster.NewClusterMetricsMgr(ctx, client, mcOpts.ClusterMetricsInterval)
		if err != nil {
			klog.ErrorS(err, "failed to enable multi-cluster-metrics capability")
			return err
		}
	}

	return nil
}

// configureFeatureGates sets up feature-dependent configurations
func configureFeatureGates(s *options.CoreOptions) {
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.ApplyOnce) {
		commonconfig.ApplicationReSyncPeriod = s.Kubernetes.InformerSyncPeriod
	}
}

// createControllerManager creates and configures the controller-runtime manager
func createControllerManager(ctx context.Context, restConfig *rest.Config, s *options.CoreOptions) (ctrl.Manager, error) {
	leaderElectionID := util.GenerateLeaderElectionID(types.KubeVelaName, s.Controller.IgnoreAppWithoutControllerRequirement)
	leaderElectionID += sharding.GetShardIDSuffix()

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: s.Observability.MetricsAddr,
		},
		LeaderElection:          s.Server.EnableLeaderElection,
		LeaderElectionNamespace: s.Server.LeaderElectionNamespace,
		LeaderElectionID:        leaderElectionID,
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port:    s.Webhook.WebhookPort,
			CertDir: s.Webhook.CertDir,
		}),
		HealthProbeBindAddress: s.Server.HealthAddr,
		LeaseDuration:          &s.Server.LeaseDuration,
		RenewDeadline:          &s.Server.RenewDeadline,
		RetryPeriod:            &s.Server.RetryPeriod,
		NewClient:              velaclient.DefaultNewControllerClient,
		NewCache: cache.BuildCache(ctx,
			ctrlcache.Options{
				Scheme:     scheme,
				SyncPeriod: &s.Kubernetes.InformerSyncPeriod,
				// SyncPeriod is configured with default value, aka. 10h. First, controller-runtime does not
				// recommend use it as a time trigger, instead, it is expected to work for failure tolerance
				// of controller-runtime. Additionally, set this value will affect not only application
				// controller but also all other controllers like definition controller. Therefore, for
				// functionalities like state-keep, they should be invented in other ways.
			},
			&v1beta1.Application{}, &v1beta1.ApplicationRevision{}, &v1beta1.ResourceTracker{},
		),
		Client: ctrlclient.Options{
			Cache: &ctrlclient.CacheOptions{
				DisableFor: cache.NewResourcesToDisableCache(),
			},
		},
	})

	if err != nil {
		klog.ErrorS(err, "Unable to create a controller manager")
		return nil, err
	}

	return mgr, nil
}

// setupControllers sets up controllers based on sharding configuration
func setupControllers(ctx context.Context, mgr ctrl.Manager, s *options.CoreOptions) error {
	if !sharding.EnableSharding {
		return prepareRun(ctx, mgr, s)
	}
	return prepareRunInShardingMode(ctx, mgr, s)
}

// startApplicationMonitor starts the application metrics watcher
func startApplicationMonitor(ctx context.Context, mgr ctrl.Manager) error {
	klog.Info("Start the vela application monitor")
	informer, err := mgr.GetCache().GetInformer(ctx, &v1beta1.Application{})
	if err != nil {
		klog.ErrorS(err, "Unable to get informer for application")
		return err
	}
	watcher.StartApplicationMetricsWatcher(informer)
	return nil
}

// performCleanup handles any necessary cleanup operations
func performCleanup(s *options.CoreOptions) {
	if s.Observability.LogFilePath != "" {
		klog.Flush()
	}
}

// prepareRunInShardingMode initializes the controller manager in sharding mode where workload
// is distributed across multiple controller instances. In sharding mode:
// - Master shard handles webhooks, scheduling, and full controller setup
// - Non-master shards only run the Application controller for their assigned Applications
// This enables horizontal scaling of the KubeVela control plane across multiple pods.
func prepareRunInShardingMode(ctx context.Context, mgr manager.Manager, s *options.CoreOptions) error {
	if sharding.IsMaster() {
		klog.Infof("controller running in sharding mode, current shard is master")
		if !utilfeature.DefaultMutableFeatureGate.Enabled(features.DisableWebhookAutoSchedule) {
			go sharding.DefaultScheduler.Get().Start(ctx)
		}
		if err := prepareRun(ctx, mgr, s); err != nil {
			return err
		}
	} else {
		klog.Infof("controller running in sharding mode, current shard id: %s", sharding.ShardID)
		if err := application.Setup(mgr, s.Controller.Args); err != nil {
			return err
		}
	}

	return nil
}

// prepareRun sets up the complete KubeVela controller manager with all necessary components:
// - Configures and registers OAM webhooks if enabled
// - Sets up all OAM controllers (Application, ComponentDefinition, WorkflowStepDefinition, PolicyDefinition, and TraitDefinition)
// - Initializes multi-cluster capabilities and cluster info
// - Runs pre-start validation hooks to ensure system readiness
// This function is used in single-instance mode or by the master shard in sharding mode.
func prepareRun(ctx context.Context, mgr manager.Manager, s *options.CoreOptions) error {
	if s.Webhook.UseWebhook {
		klog.InfoS("Enable webhook", "server port", strconv.Itoa(s.Webhook.WebhookPort))
		oamwebhook.Register(mgr, s.Controller.Args)
		if err := waitWebhookSecretVolume(s.Webhook.CertDir, waitSecretTimeout, waitSecretInterval); err != nil {
			klog.ErrorS(err, "Unable to get webhook secret")
			return err
		}
	}

	if err := oamv1beta1.Setup(mgr, s.Controller.Args); err != nil {
		klog.ErrorS(err, "Unable to setup the oam controller")
		return err
	}

	if err := multicluster.InitClusterInfo(mgr.GetConfig()); err != nil {
		klog.ErrorS(err, "Init control plane cluster info")
		return err
	}

	klog.Info("Start the vela controller manager")
	for _, hook := range []hooks.PreStartHook{hooks.NewSystemCRDValidationHook()} {
		if err := hook.Run(ctx); err != nil {
			return fmt.Errorf("failed to run hook %T: %w", hook, err)
		}
	}

	return nil
}

// registerHealthChecks is used to create readiness&liveness probes
func registerHealthChecks(mgr ctrl.Manager) error {
	klog.Info("Create readiness/health check")
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	// TODO: change the health check to be different from readiness check
	return mgr.AddHealthzCheck("ping", healthz.Ping)
}

// waitWebhookSecretVolume waits for webhook secret ready to avoid mgr running crash
func waitWebhookSecretVolume(certDir string, timeout, interval time.Duration) error {
	start := time.Now()
	for {
		time.Sleep(interval)
		if time.Since(start) > timeout {
			return fmt.Errorf("getting webhook secret timeout after %s", timeout.String())
		}
		klog.InfoS("Wait webhook secret", "time consumed(second)", int64(time.Since(start).Seconds()),
			"timeout(second)", int64(timeout.Seconds()))
		if _, err := os.Stat(certDir); !os.IsNotExist(err) {
			ready := func() bool {
				f, err := os.Open(filepath.Clean(certDir))
				if err != nil {
					return false
				}
				defer func() {
					if err := f.Close(); err != nil {
						klog.Error(err, "Failed to close file")
					}
				}()
				// check if dir is empty
				if _, err := f.Readdir(1); errors.Is(err, io.EOF) {
					return false
				}
				// check if secret files are empty
				err = filepath.Walk(certDir, func(path string, info os.FileInfo, err error) error {
					// even Cert dir is created, cert files are still empty for a while
					if info.Size() == 0 {
						return errors.New("secret is not ready")
					}
					return nil
				})
				if err == nil {
					klog.InfoS("Webhook secret is ready", "time consumed(second)",
						int64(time.Since(start).Seconds()))
					return true
				}
				return false
			}()
			if ready {
				return nil
			}
		}
	}
}
