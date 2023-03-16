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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	velaclient "github.com/kubevela/pkg/controller/client"
	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/meta"
	"github.com/kubevela/workflow/pkg/cue/packages"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/cmd/core/app/hooks"
	"github.com/oam-dev/kubevela/cmd/core/app/options"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cache"
	standardcontroller "github.com/oam-dev/kubevela/pkg/controller"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	oamv1alpha2 "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/watcher"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	pkgutil "github.com/oam-dev/kubevela/pkg/utils"
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
		Use: "vela-core",
		Long: `The KubeVela controller manager is a daemon that embeds
the core control loops shipped with KubeVela`,
		RunE: func(cmd *cobra.Command, args []string) error {

			return run(signals.SetupSignalHandler(), s)
		},
		SilenceUsage: true,
	}

	fs := cmd.Flags()
	namedFlagSets := s.Flags()
	for _, set := range namedFlagSets.FlagSets {
		fs.AddFlagSet(set)
	}
	meta.Name = types.VelaCoreName

	klog.InfoS("KubeVela information", "version", version.VelaVersion, "revision", version.GitRevision)
	klog.InfoS("Disable capabilities", "name", s.DisableCaps)
	klog.InfoS("Vela-Core init", "definition namespace", oam.SystemDefinitionNamespace)

	return cmd
}

func run(ctx context.Context, s *options.CoreOptions) error {
	klog.InfoS("KubeVela information", "version", version.VelaVersion, "revision", version.GitRevision)
	klog.InfoS("Disable capabilities", "name", s.DisableCaps)
	klog.InfoS("Vela-Core init", "definition namespace", oam.SystemDefinitionNamespace)

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = types.KubeVelaName + "/" + version.GitRevision
	restConfig.QPS = float32(s.QPS)
	restConfig.Burst = s.Burst
	restConfig.Wrap(auth.NewImpersonatingRoundTripper)
	klog.InfoS("Kubernetes Config Loaded",
		"UserAgent", restConfig.UserAgent,
		"QPS", restConfig.QPS,
		"Burst", restConfig.Burst,
	)

	if s.PprofAddr != "" {
		go pkgutil.EnablePprof(s.PprofAddr, nil)
	}

	// wrapper the round tripper by multi cluster rewriter
	if s.EnableClusterGateway {
		client, err := multicluster.Initialize(restConfig, true)
		if err != nil {
			klog.ErrorS(err, "failed to enable multi-cluster capability")
			return err
		}

		if s.EnableClusterMetrics {
			_, err := multicluster.NewClusterMetricsMgr(context.Background(), client, s.ClusterMetricsInterval)
			if err != nil {
				klog.ErrorS(err, "failed to enable multi-cluster-metrics capability")
				return err
			}
		}
	}

	ctrl.SetLogger(klogr.New())

	if utilfeature.DefaultMutableFeatureGate.Enabled(features.ApplyOnce) {
		commonconfig.ApplicationReSyncPeriod = s.InformerSyncPeriod
	}

	leaderElectionID := util.GenerateLeaderElectionID(types.KubeVelaName, s.ControllerArgs.IgnoreAppWithoutControllerRequirement)
	leaderElectionID += sharding.GetShardIDSuffix()
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         s.MetricsAddr,
		LeaderElection:             s.EnableLeaderElection,
		LeaderElectionNamespace:    s.LeaderElectionNamespace,
		LeaderElectionID:           leaderElectionID,
		Port:                       s.WebhookPort,
		CertDir:                    s.CertDir,
		HealthProbeBindAddress:     s.HealthAddr,
		LeaderElectionResourceLock: s.LeaderElectionResourceLock,
		LeaseDuration:              &s.LeaseDuration,
		RenewDeadline:              &s.RenewDeadLine,
		RetryPeriod:                &s.RetryPeriod,
		SyncPeriod:                 &s.InformerSyncPeriod,
		// SyncPeriod is configured with default value, aka. 10h. First, controller-runtime does not
		// recommend use it as a time trigger, instead, it is expected to work for failure tolerance
		// of controller-runtime. Additionally, set this value will affect not only application
		// controller but also all other controllers like definition controller. Therefore, for
		// functionalities like state-keep, they should be invented in other ways.
		NewClient:             velaclient.DefaultNewControllerClient,
		NewCache:              cache.BuildCache(ctx, scheme, &v1beta1.Application{}, &v1beta1.ApplicationRevision{}, &v1beta1.ResourceTracker{}),
		ClientDisableCacheFor: cache.NewResourcesToDisableCache(),
	})
	if err != nil {
		klog.ErrorS(err, "Unable to create a controller manager")
		return err
	}

	if err := registerHealthChecks(mgr); err != nil {
		klog.ErrorS(err, "Unable to register ready/health checks")
		return err
	}

	if err := utils.CheckDisabledCapabilities(s.DisableCaps); err != nil {
		klog.ErrorS(err, "Unable to get enabled capabilities")
		return err
	}

	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		klog.ErrorS(err, "Failed to create CRD discovery client")
		return err
	}
	s.ControllerArgs.DiscoveryMapper = dm
	pd, err := packages.NewPackageDiscover(mgr.GetConfig())
	if err != nil {
		klog.Error(err, "Failed to create CRD discovery for CUE package client")
		if !packages.IsCUEParseErr(err) {
			return err
		}
	}
	s.ControllerArgs.PackageDiscover = pd

	if !sharding.EnableSharding {
		if err = prepareRun(ctx, mgr, s); err != nil {
			return err
		}
	} else {
		if err = prepareRunInShardingMode(ctx, mgr, s); err != nil {
			return err
		}
	}

	klog.Info("Start the vela application monitor")
	informer, err := mgr.GetCache().GetInformer(ctx, &v1beta1.Application{})
	if err != nil {
		klog.ErrorS(err, "Unable to get informer for application")
	}
	watcher.StartApplicationMetricsWatcher(informer)

	if err := mgr.Start(ctx); err != nil {
		klog.ErrorS(err, "Failed to run manager")
		return err
	}
	if s.LogFilePath != "" {
		klog.Flush()
	}
	klog.Info("Safely stops Program...")
	return nil
}

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
		if err := application.Setup(mgr, *s.ControllerArgs); err != nil {
			return err
		}
	}

	return nil
}

func prepareRun(ctx context.Context, mgr manager.Manager, s *options.CoreOptions) error {
	if s.UseWebhook {
		klog.InfoS("Enable webhook", "server port", strconv.Itoa(s.WebhookPort))
		oamwebhook.Register(mgr, *s.ControllerArgs)
		if err := waitWebhookSecretVolume(s.CertDir, waitSecretTimeout, waitSecretInterval); err != nil {
			klog.ErrorS(err, "Unable to get webhook secret")
			return err
		}
	}

	if err := oamv1alpha2.Setup(mgr, *s.ControllerArgs); err != nil {
		klog.ErrorS(err, "Unable to setup the oam controller")
		return err
	}

	if err := standardcontroller.Setup(mgr, s.DisableCaps, *s.ControllerArgs); err != nil {
		klog.ErrorS(err, "Unable to setup the vela core controller")
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
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	return nil
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
