package main

import (
	"flag"
	"io"
	"os"
	"strconv"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	controller "github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2"
	webhook "github.com/crossplane/oam-kubernetes-runtime/pkg/webhook/v1alpha2"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr, logFilePath string
	var enableLeaderElection, logCompress bool
	var logRetainDate int
	var certDir string
	var webhookPort int
	var useWebhook bool

	flag.BoolVar(&useWebhook, "use-webhook", false, "Enable Admission Webhook")
	flag.StringVar(&certDir, "webhook-cert-dir", "/k8s-webhook-server/serving-certs", "Admission webhook cert/key dir.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "admission webhook listen address")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&logFilePath, "log-file-path", "", "The address the metric endpoint binds to.")
	flag.IntVar(&logRetainDate, "log-retain-date", 7, "The number of days of logs history to retain.")
	flag.BoolVar(&logCompress, "log-compress", true, "Enable compression on the rotated logs.")
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

	oamLog := ctrl.Log.WithName("vela-core")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "vela-core",
		Port:               webhookPort,
		CertDir:            certDir,
	})
	if err != nil {
		oamLog.Error(err, "unable to create a controller manager")
		os.Exit(1)
	}

	if useWebhook {
		oamLog.Info("OAM webhook enabled, will serving at :" + strconv.Itoa(webhookPort))
		webhook.Add(mgr)
	}

	if err = controller.Setup(mgr, logging.NewLogrLogger(oamLog)); err != nil {
		oamLog.Error(err, "unable to setup the oam core controller")
		os.Exit(1)
	}
	oamLog.Info("starting the controller manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		oamLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
