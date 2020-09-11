package main

import (
	"flag"
	"io"
	"os"
	"strconv"

	monitoring "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	oamcore "github.com/crossplane/oam-kubernetes-runtime/apis/core"
	oamcontroller "github.com/crossplane/oam-kubernetes-runtime/pkg/controller"
	oamv1alpha2 "github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2"
	oamwebhook "github.com/crossplane/oam-kubernetes-runtime/pkg/webhook/v1alpha2"
	injectorv1alpha1 "github.com/oam-dev/trait-injector/api/v1alpha1"
	injectorcontroller "github.com/oam-dev/trait-injector/controllers"
	"github.com/oam-dev/trait-injector/pkg/injector"
	"github.com/oam-dev/trait-injector/pkg/plugin"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	velacore "github.com/oam-dev/kubevela/api/v1alpha1"
	velacontroller "github.com/oam-dev/kubevela/pkg/controller"
	velawebhook "github.com/oam-dev/kubevela/pkg/webhook"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = oamcore.AddToScheme(scheme)
	_ = monitoring.AddToScheme(scheme)
	_ = velacore.AddToScheme(scheme)
	_ = injectorv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr, logFilePath string
	var enableLeaderElection, logCompress bool
	var logRetainDate int
	var certDir string
	var webhookPort int
	var useWebhook, useTraitInjector bool
	var controllerArgs oamcontroller.Args

	flag.BoolVar(&useWebhook, "use-webhook", false, "Enable Admission Webhook")
	flag.BoolVar(&useTraitInjector, "use-trait-injector", false, "Enable TraitInjector")
	flag.StringVar(&certDir, "webhook-cert-dir", "/k8s-webhook-server/serving-certs", "Admission webhook cert/key dir.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "admission webhook listen address")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&logFilePath, "log-file-path", "", "The address the metric endpoint binds to.")
	flag.IntVar(&logRetainDate, "log-retain-date", 7, "The number of days of logs history to retain.")
	flag.BoolVar(&logCompress, "log-compress", true, "Enable compression on the rotated logs.")
	flag.IntVar(&controllerArgs.RevisionLimit, "revision-limit", 50,
		"RevisionLimit is the maximum number of revisions that will be maintained. The default value is 50.")
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

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
		o.DestWritter = w
	}))

	setupLog := ctrl.Log.WithName("vela-runtime")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "vela-runtime",
		Port:               webhookPort,
		CertDir:            certDir,
	})
	if err != nil {
		setupLog.Error(err, "unable to create a controller manager")
		os.Exit(1)
	}

	if useWebhook {
		setupLog.Info("vela webhook enabled, will serving at :" + strconv.Itoa(webhookPort))
		oamwebhook.Add(mgr)
		velawebhook.Register(mgr)
	}

	if err = oamv1alpha2.Setup(mgr, controllerArgs, logging.NewLogrLogger(setupLog)); err != nil {
		setupLog.Error(err, "unable to setup the oam core controller")
		os.Exit(1)
	}

	if err = velacontroller.Setup(mgr); err != nil {
		setupLog.Error(err, "unable to setup the vela core controller")
		os.Exit(1)
	}

	if useTraitInjector {
		// register all service injectors
		plugin.RegisterTargetInjectors(injector.Defaults()...)

		tiWebhook := &injectorcontroller.ServiceBindingReconciler{
			Client:   mgr.GetClient(),
			Log:      ctrl.Log.WithName("controllers").WithName("ServiceBinding"),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("servicebinding"),
		}
		if err = (tiWebhook).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ServiceBinding")
			os.Exit(1)
		}
		// this has hard coded requirement "./ssl/service-injector.pem", "./ssl/service-injector.key"
		go tiWebhook.ServeAdmission()
	}

	setupLog.Info("starting the vela controller manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
