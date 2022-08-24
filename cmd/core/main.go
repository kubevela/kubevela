/*
Copyright 2021 The KubeVela Authors.

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

package main

import (
	"context"
	"errors"
	goflag "flag"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	flag "github.com/spf13/pflag"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	ctrlClient "github.com/oam-dev/kubevela/pkg/client"
	standardcontroller "github.com/oam-dev/kubevela/pkg/controller"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	oamcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	oamv1alpha2 "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	_ "github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/monitor/watcher"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	oamwebhook "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
	"github.com/oam-dev/kubevela/version"
)

var (
	scheme             = common.Scheme
	waitSecretTimeout  = 90 * time.Second
	waitSecretInterval = 2 * time.Second
)

func main() {
	var metricsAddr, logFilePath, leaderElectionNamespace string
	var enableLeaderElection, logDebug bool
	var logFileMaxSize uint64
	var certDir string
	var webhookPort int
	var useWebhook bool
	var controllerArgs oamcontroller.Args
	var healthAddr string
	var disableCaps string
	var storageDriver string
	var applyOnceOnly string
	var qps float64
	var burst int
	var pprofAddr string
	var leaderElectionResourceLock string
	var leaseDuration time.Duration
	var renewDeadline time.Duration
	var retryPeriod time.Duration
	var enableClusterGateway bool
	var enableClusterMetrics bool
	var clusterMetricsInterval time.Duration

	flag.BoolVar(&useWebhook, "use-webhook", false, "Enable Admission Webhook")
	flag.StringVar(&certDir, "webhook-cert-dir", "/k8s-webhook-server/serving-certs", "Admission webhook cert/key dir.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "admission webhook listen address")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "",
		"Determines the namespace in which the leader election configmap will be created.")
	flag.StringVar(&logFilePath, "log-file-path", "", "The file to write logs to.")
	flag.Uint64Var(&logFileMaxSize, "log-file-max-size", 1024, "Defines the maximum size a log file can grow to, Unit is megabytes.")
	flag.BoolVar(&logDebug, "log-debug", false, "Enable debug logs for development purpose")
	flag.IntVar(&controllerArgs.RevisionLimit, "revision-limit", 50,
		"RevisionLimit is the maximum number of revisions that will be maintained. The default value is 50.")
	flag.IntVar(&controllerArgs.AppRevisionLimit, "application-revision-limit", 10,
		"application-revision-limit is the maximum number of application useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 10.")
	flag.IntVar(&controllerArgs.DefRevisionLimit, "definition-revision-limit", 20,
		"definition-revision-limit is the maximum number of component/trait definition useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 20.")
	flag.StringVar(&controllerArgs.CustomRevisionHookURL, "custom-revision-hook-url", "",
		"custom-revision-hook-url is a webhook url which will let KubeVela core to call with applicationConfiguration and component info and return a customized component revision")
	flag.BoolVar(&controllerArgs.AutoGenWorkloadDefinition, "autogen-workload-definition", true, "Automatic generated workloadDefinition which componentDefinition refers to.")
	flag.StringVar(&healthAddr, "health-addr", ":9440", "The address the health endpoint binds to.")
	flag.StringVar(&applyOnceOnly, "apply-once-only", "false",
		"For the purpose of some production environment that workload or trait should not be affected if no spec change, available options: on, off, force.")
	flag.StringVar(&disableCaps, "disable-caps", "", "To be disabled builtin capability list.")
	flag.StringVar(&storageDriver, "storage-driver", "Local", "Application file save to the storage driver")
	flag.DurationVar(&commonconfig.ApplicationReSyncPeriod, "application-re-sync-period", 5*time.Minute,
		"Re-sync period for application to re-sync, also known as the state-keep interval.")
	flag.DurationVar(&commonconfig.ReconcileTimeout, "reconcile-timeout", time.Minute*3,
		"the timeout for controller reconcile")
	flag.StringVar(&oam.SystemDefinitonNamespace, "system-definition-namespace", "vela-system", "define the namespace of the system-level definition")
	flag.IntVar(&controllerArgs.ConcurrentReconciles, "concurrent-reconciles", 4, "concurrent-reconciles is the concurrent reconcile number of the controller. The default value is 4")
	flag.Float64Var(&qps, "kube-api-qps", 50, "the qps for reconcile clients. Low qps may lead to low throughput. High qps may give stress to api-server. Raise this value if concurrent-reconciles is set to be high.")
	flag.IntVar(&burst, "kube-api-burst", 100, "the burst for reconcile clients. Recommend setting it qps*2.")
	flag.DurationVar(&controllerArgs.DependCheckWait, "depend-check-wait", 30*time.Second, "depend-check-wait is the time to wait for ApplicationConfiguration's dependent-resource ready."+
		"The default value is 30s, which means if dependent resources were not prepared, the ApplicationConfiguration would be reconciled after 30s.")
	flag.StringVar(&controllerArgs.OAMSpecVer, "oam-spec-ver", "v0.3", "oam-spec-ver is the oam spec version controller want to setup, available options: v0.2, v0.3, all")
	flag.StringVar(&pprofAddr, "pprof-addr", "", "The address for pprof to use while exporting profiling results. The default value is empty which means do not expose it. Set it to address like :6666 to expose it.")
	flag.BoolVar(&commonconfig.PerfEnabled, "perf-enabled", false, "Enable performance logging for controllers, disabled by default.")
	flag.StringVar(&leaderElectionResourceLock, "leader-election-resource-lock", "configmapsleases", "The resource lock to use for leader election")
	flag.DurationVar(&leaseDuration, "leader-election-lease-duration", 15*time.Second,
		"The duration that non-leader candidates will wait to force acquire leadership")
	flag.DurationVar(&renewDeadline, "leader-election-renew-deadline", 10*time.Second,
		"The duration that the acting controlplane will retry refreshing leadership before giving up")
	flag.DurationVar(&retryPeriod, "leader-election-retry-period", 2*time.Second,
		"The duration the LeaderElector clients should wait between tries of actions")
	flag.BoolVar(&enableClusterGateway, "enable-cluster-gateway", false, "Enable cluster-gateway to use multicluster, disabled by default.")
	flag.BoolVar(&enableClusterMetrics, "enable-cluster-metrics", false, "Enable cluster-metrics-management to collect metrics from clusters with cluster-gateway, disabled by default. When this param is enabled, enable-cluster-gateway should be enabled")
	flag.DurationVar(&clusterMetricsInterval, "cluster-metrics-interval", 15*time.Second, "The interval that ClusterMetricsMgr will collect metrics from clusters, default value is 15 seconds.")
	flag.BoolVar(&controllerArgs.EnableCompatibility, "enable-asi-compatibility", false, "enable compatibility for asi")
	flag.BoolVar(&controllerArgs.IgnoreAppWithoutControllerRequirement, "ignore-app-without-controller-version", false, "If true, application controller will not process the app without 'app.oam.dev/controller-version-require' annotation")
	flag.BoolVar(&controllerArgs.IgnoreDefinitionWithoutControllerRequirement, "ignore-definition-without-controller-version", false, "If true, trait/component/workflowstep definition controller will not process the definition without 'definition.oam.dev/controller-version-require' annotation")
	standardcontroller.AddOptimizeFlags()
	standardcontroller.AddAdmissionFlags()
	flag.IntVar(&resourcekeeper.MaxDispatchConcurrent, "max-dispatch-concurrent", 10, "Set the max dispatch concurrent number, default is 10")
	flag.IntVar(&wfTypes.MaxWorkflowWaitBackoffTime, "max-workflow-wait-backoff-time", 60, "Set the max workflow wait backoff time, default is 60")
	flag.IntVar(&wfTypes.MaxWorkflowFailedBackoffTime, "max-workflow-failed-backoff-time", 300, "Set the max workflow wait backoff time, default is 300")
	flag.IntVar(&wfTypes.MaxWorkflowStepErrorRetryTimes, "max-workflow-step-error-retry-times", 10, "Set the max workflow step error retry times, default is 10")
	utilfeature.DefaultMutableFeatureGate.AddFlag(flag.CommandLine)

	// setup logging
	klog.InitFlags(nil)
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	flag.Parse()
	if logDebug {
		_ = flag.Set("v", strconv.Itoa(int(commonconfig.LogDebug)))
	}

	if pprofAddr != "" {
		// Start pprof server if enabled
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		pprofServer := http.Server{
			Addr:    pprofAddr,
			Handler: mux,
		}
		klog.InfoS("Starting debug HTTP server", "addr", pprofServer.Addr)

		go func() {
			go func() {
				ctx := context.Background()
				<-ctx.Done()

				ctx, cancelFunc := context.WithTimeout(context.Background(), 60*time.Minute)
				defer cancelFunc()

				if err := pprofServer.Shutdown(ctx); err != nil {
					klog.Error(err, "Failed to shutdown debug HTTP server")
				}
			}()

			if err := pprofServer.ListenAndServe(); !errors.Is(http.ErrServerClosed, err) {
				klog.Error(err, "Failed to start debug HTTP server")
				panic(err)
			}
		}()
	}

	if logFilePath != "" {
		_ = flag.Set("logtostderr", "false")
		_ = flag.Set("log_file", logFilePath)
		_ = flag.Set("log_file_max_size", strconv.FormatUint(logFileMaxSize, 10))
	}

	klog.InfoS("KubeVela information", "version", version.VelaVersion, "revision", version.GitRevision)
	klog.InfoS("Disable capabilities", "name", disableCaps)
	klog.InfoS("Vela-Core init", "definition namespace", oam.SystemDefinitonNamespace)

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = types.KubeVelaName + "/" + version.GitRevision
	restConfig.QPS = float32(qps)
	restConfig.Burst = burst
	restConfig.Wrap(auth.NewImpersonatingRoundTripper)
	klog.InfoS("Kubernetes Config Loaded",
		"UserAgent", restConfig.UserAgent,
		"QPS", restConfig.QPS,
		"Burst", restConfig.Burst,
	)

	// wrapper the round tripper by multi cluster rewriter
	if enableClusterGateway {
		client, err := multicluster.Initialize(restConfig, true)
		if err != nil {
			klog.ErrorS(err, "failed to enable multi-cluster capability")
			os.Exit(1)
		}

		if enableClusterMetrics {
			_, err := multicluster.NewClusterMetricsMgr(context.Background(), client, clusterMetricsInterval)
			if err != nil {
				klog.ErrorS(err, "failed to enable multi-cluster-metrics capability")
				os.Exit(1)
			}
		}
	}
	ctrl.SetLogger(klogr.New())

	leaderElectionID := util.GenerateLeaderElectionID(types.KubeVelaName, controllerArgs.IgnoreAppWithoutControllerRequirement)
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                     scheme,
		MetricsBindAddress:         metricsAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionNamespace:    leaderElectionNamespace,
		LeaderElectionID:           leaderElectionID,
		Port:                       webhookPort,
		CertDir:                    certDir,
		HealthProbeBindAddress:     healthAddr,
		LeaderElectionResourceLock: leaderElectionResourceLock,
		LeaseDuration:              &leaseDuration,
		RenewDeadline:              &renewDeadline,
		RetryPeriod:                &retryPeriod,
		// SyncPeriod is configured with default value, aka. 10h. First, controller-runtime does not
		// recommend use it as a time trigger, instead, it is expected to work for failure tolerance
		// of controller-runtime. Additionally, set this value will affect not only application
		// controller but also all other controllers like definition controller. Therefore, for
		// functionalities like state-keep, they should be invented in other ways.
		NewClient: ctrlClient.DefaultNewControllerClient,
	})
	if err != nil {
		klog.ErrorS(err, "Unable to create a controller manager")
		os.Exit(1)
	}

	if err := registerHealthChecks(mgr); err != nil {
		klog.ErrorS(err, "Unable to register ready/health checks")
		os.Exit(1)
	}

	if err := utils.CheckDisabledCapabilities(disableCaps); err != nil {
		klog.ErrorS(err, "Unable to get enabled capabilities")
		os.Exit(1)
	}

	switch strings.ToLower(applyOnceOnly) {
	case "", "false", string(oamcontroller.ApplyOnceOnlyOff):
		controllerArgs.ApplyMode = oamcontroller.ApplyOnceOnlyOff
		klog.Info("ApplyOnceOnly is disabled")
	case "true", string(oamcontroller.ApplyOnceOnlyOn):
		controllerArgs.ApplyMode = oamcontroller.ApplyOnceOnlyOn
		klog.Info("ApplyOnceOnly is enabled, that means workload or trait only apply once if no spec change even they are changed by others")
	case string(oamcontroller.ApplyOnceOnlyForce):
		controllerArgs.ApplyMode = oamcontroller.ApplyOnceOnlyForce
		klog.Info("ApplyOnceOnlyForce is enabled, that means workload or trait only apply once if no spec change even they are changed or deleted by others")
	default:
		klog.ErrorS(fmt.Errorf("invalid apply-once-only value: %s", applyOnceOnly),
			"Unable to setup the vela core controller",
			"apply-once-only", "on/off/force, by default it's off")
		os.Exit(1)
	}

	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		klog.ErrorS(err, "Failed to create CRD discovery client")
		os.Exit(1)
	}
	controllerArgs.DiscoveryMapper = dm
	pd, err := packages.NewPackageDiscover(mgr.GetConfig())
	if err != nil {
		klog.Error(err, "Failed to create CRD discovery for CUE package client")
		if !packages.IsCUEParseErr(err) {
			os.Exit(1)
		}
	}
	controllerArgs.PackageDiscover = pd

	if useWebhook {
		klog.InfoS("Enable webhook", "server port", strconv.Itoa(webhookPort))
		oamwebhook.Register(mgr, controllerArgs)
		if err := waitWebhookSecretVolume(certDir, waitSecretTimeout, waitSecretInterval); err != nil {
			klog.ErrorS(err, "Unable to get webhook secret")
			os.Exit(1)
		}
	}

	if err = oamv1alpha2.Setup(mgr, controllerArgs); err != nil {
		klog.ErrorS(err, "Unable to setup the oam controller")
		os.Exit(1)
	}

	if err = standardcontroller.Setup(mgr, disableCaps, controllerArgs); err != nil {
		klog.ErrorS(err, "Unable to setup the vela core controller")
		os.Exit(1)
	}

	if driver := os.Getenv(system.StorageDriverEnv); len(driver) == 0 {
		// first use system environment,
		err := os.Setenv(system.StorageDriverEnv, storageDriver)
		if err != nil {
			klog.ErrorS(err, "Unable to setup the vela core controller")
			os.Exit(1)
		}
	}
	klog.InfoS("Use storage driver", "storageDriver", os.Getenv(system.StorageDriverEnv))

	klog.Info("Start the vela application monitor")
	informer, err := mgr.GetCache().GetInformer(context.Background(), &v1beta1.Application{})
	if err != nil {
		klog.ErrorS(err, "Unable to get informer for application")
	}
	watcher.StartApplicationMetricsWatcher(informer)

	klog.Info("Start the vela controller manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.ErrorS(err, "Failed to run manager")
		os.Exit(1)
	}
	if logFilePath != "" {
		klog.Flush()
	}
	klog.Info("Safely stops Program...")
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
