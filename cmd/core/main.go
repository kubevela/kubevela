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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	standardcontroller "github.com/oam-dev/kubevela/pkg/controller"
	oamcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	oamv1alpha2 "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	oamwebhook "github.com/oam-dev/kubevela/pkg/webhook/core.oam.dev"
	velawebhook "github.com/oam-dev/kubevela/pkg/webhook/standard.oam.dev"
	"github.com/oam-dev/kubevela/version"
)

const (
	kubevelaName = "kubevela"
)

var (
	setupLog           = ctrl.Log.WithName(kubevelaName)
	scheme             = common.Scheme
	waitSecretTimeout  = 90 * time.Second
	waitSecretInterval = 2 * time.Second
)

func main() {
	var metricsAddr, logFilePath, leaderElectionNamespace string
	var enableLeaderElection, logCompress, logDebug bool
	var logRetainDate int
	var certDir string
	var webhookPort int
	var useWebhook bool
	var controllerArgs oamcontroller.Args
	var healthAddr string
	var disableCaps string
	var storageDriver string
	var syncPeriod time.Duration
	var applyOnceOnly string

	flag.BoolVar(&useWebhook, "use-webhook", false, "Enable Admission Webhook")
	flag.StringVar(&certDir, "webhook-cert-dir", "/k8s-webhook-server/serving-certs", "Admission webhook cert/key dir.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "admission webhook listen address")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "",
		"Determines the namespace in which the leader election configmap will be created.")
	flag.StringVar(&logFilePath, "log-file-path", "", "The file to write logs to.")
	flag.IntVar(&logRetainDate, "log-retain-date", 7, "The number of days of logs history to retain.")
	flag.BoolVar(&logCompress, "log-compress", true, "Enable compression on the rotated logs.")
	flag.BoolVar(&logDebug, "log-debug", false, "Enable debug logs for development purpose")
	flag.IntVar(&controllerArgs.RevisionLimit, "revision-limit", 50,
		"RevisionLimit is the maximum number of revisions that will be maintained. The default value is 50.")
	flag.IntVar(&controllerArgs.AppRevisionLimit, "application-revision-limit", 10,
		"application-revision-limit is the maximum number of application useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 10.")
	flag.IntVar(&controllerArgs.DefRevisionLimit, "definition-revision-limit", 20,
		"definition-revision-limit is the maximum number of component/trait definition useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 20.")
	flag.StringVar(&controllerArgs.CustomRevisionHookURL, "custom-revision-hook-url", "",
		"custom-revision-hook-url is a webhook url which will let KubeVela core to call with applicationConfiguration and component info and return a customized component revision")
	flag.BoolVar(&controllerArgs.ApplicationConfigurationInstalled, "app-config-installed", true,
		"app-config-installed indicates if applicationConfiguration CRD is installed")
	flag.StringVar(&healthAddr, "health-addr", ":9440", "The address the health endpoint binds to.")
	flag.StringVar(&applyOnceOnly, "apply-once-only", "false",
		"For the purpose of some production environment that workload or trait should not be affected if no spec change, available options: on, off, force.")
	flag.StringVar(&disableCaps, "disable-caps", "", "To be disabled builtin capability list.")
	flag.StringVar(&storageDriver, "storage-driver", "Local", "Application file save to the storage driver")
	flag.DurationVar(&syncPeriod, "informer-re-sync-interval", 60*time.Minute,
		"controller shared informer lister full re-sync period")
	flag.StringVar(&oam.SystemDefinitonNamespace, "system-definition-namespace", "vela-system", "define the namespace of the system-level definition")
	flag.Parse()

	// setup logging
	var w io.Writer
	if len(logFilePath) > 0 {
		w = zapcore.AddSync(&lumberjack.Logger{
			Filename: logFilePath,
			MaxAge:   logRetainDate, // days
			Compress: logCompress,
		})
	} else {
		w = os.Stdout
	}

	logger := zap.New(func(o *zap.Options) {
		o.Development = logDebug
		o.DestWritter = w
	})
	ctrl.SetLogger(logger)

	setupLog.Info(fmt.Sprintf("KubeVela Version: %s, GIT Revision: %s.", version.VelaVersion, version.GitRevision))
	setupLog.Info(fmt.Sprintf("Disable Capabilities: %s.", disableCaps))
	setupLog.Info(fmt.Sprintf("core init with definition namespace %s", oam.SystemDefinitonNamespace))

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = kubevelaName + "/" + version.GitRevision

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionNamespace: leaderElectionNamespace,
		LeaderElectionID:        kubevelaName,
		Port:                    webhookPort,
		CertDir:                 certDir,
		HealthProbeBindAddress:  healthAddr,
		SyncPeriod:              &syncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to create a controller manager")
		os.Exit(1)
	}

	if err := registerHealthChecks(mgr); err != nil {
		setupLog.Error(err, "unable to register ready/health checks")
		os.Exit(1)
	}

	if err := utils.CheckDisabledCapabilities(disableCaps); err != nil {
		setupLog.Error(err, "unable to get enabled capabilities")
		os.Exit(1)
	}

	switch strings.ToLower(applyOnceOnly) {
	case "", "false", string(oamcontroller.ApplyOnceOnlyOff):
		controllerArgs.ApplyMode = oamcontroller.ApplyOnceOnlyOff
		setupLog.Info("ApplyOnceOnly is disabled")
	case "true", string(oamcontroller.ApplyOnceOnlyOn):
		controllerArgs.ApplyMode = oamcontroller.ApplyOnceOnlyOn
		setupLog.Info("ApplyOnceOnly is enabled, that means workload or trait only apply once if no spec change even they are changed by others")
	case string(oamcontroller.ApplyOnceOnlyForce):
		controllerArgs.ApplyMode = oamcontroller.ApplyOnceOnlyForce
		setupLog.Info("ApplyOnceOnlyForce is enabled, that means workload or trait only apply once if no spec change even they are changed or deleted by others")
	default:
		setupLog.Error(fmt.Errorf("invalid apply-once-only value: %s", applyOnceOnly),
			"unable to setup the vela core controller",
			"valid apply-once-only value:", "on/off/force, by default it's off")
		os.Exit(1)
	}

	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "failed to create CRD discovery client")
		os.Exit(1)
	}
	controllerArgs.DiscoveryMapper = dm
	pd, err := definition.NewPackageDiscover(mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "failed to create CRD discovery for CUE package client")
		os.Exit(1)
	}
	controllerArgs.PackageDiscover = pd

	if useWebhook {
		setupLog.Info("vela webhook enabled, will serving at :" + strconv.Itoa(webhookPort))
		oamwebhook.Register(mgr, controllerArgs)
		velawebhook.Register(mgr, disableCaps)
		if err := waitWebhookSecretVolume(certDir, waitSecretTimeout, waitSecretInterval); err != nil {
			setupLog.Error(err, "unable to get webhook secret")
			os.Exit(1)
		}
	}

	if err = oamv1alpha2.Setup(mgr, controllerArgs, logging.NewLogrLogger(setupLog)); err != nil {
		setupLog.Error(err, "unable to setup the oam core controller")
		os.Exit(1)
	}

	if err = standardcontroller.Setup(mgr, disableCaps); err != nil {
		setupLog.Error(err, "unable to setup the vela core controller")
		os.Exit(1)
	}
	if driver := os.Getenv(system.StorageDriverEnv); len(driver) == 0 {
		// first use system environment,
		err := os.Setenv(system.StorageDriverEnv, storageDriver)
		if err != nil {
			setupLog.Error(err, "unable to setup the vela core controller")
			os.Exit(1)
		}
	}
	setupLog.Info("use storage driver", "storageDriver", os.Getenv(system.StorageDriverEnv))

	setupLog.Info("starting the vela controller manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
	setupLog.Info("program safely stops...")
}

// registerHealthChecks is used to create readiness&liveness probes
func registerHealthChecks(mgr ctrl.Manager) error {
	setupLog.Info("creating readiness/health check")
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
		setupLog.Info(fmt.Sprintf("waiting webhook secret, time consumed: %d/%d seconds ...",
			int64(time.Since(start).Seconds()), int64(timeout.Seconds())))
		if _, err := os.Stat(certDir); !os.IsNotExist(err) {
			ready := func() bool {
				f, err := os.Open(filepath.Clean(certDir))
				if err != nil {
					return false
				}
				// nolint
				defer f.Close()
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
					setupLog.Info(fmt.Sprintf("webhook secret is ready (time consumed: %d seconds)",
						int64(time.Since(start).Seconds())))
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
