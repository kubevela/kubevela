package main

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/application"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func newServerCommand() *cobra.Command {
	options := new(serverOptions)
	cmd := &cobra.Command{
		Use:  "vela-server",
		Long: "The K8s-native Vela Controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
			options.Scheme = scheme
			options.LeaderElectionID = "vela-server"
			mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options.Options)
			if err != nil {
				setupLog.Error(err, "unable to start manager")
				return err
			}

			if err := application.Setup(mgr); err != nil {
				return err
			}

			if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
				setupLog.Error(err, "problem running manager")
				return err
			}
			return nil
		},
	}
	options.register(cmd.Flags())

	return cmd
}

type serverOptions struct {
	ctrl.Options
}

func (option *serverOptions) register(fs *pflag.FlagSet) {
	fs.StringVar(&option.MetricsBindAddress, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	fs.BoolVar(&option.LeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&option.LeaderElectionNamespace, "leader-election-namespace", "",
		"Determines the namespace in which the leader election configmap will be created.")
	option.SyncPeriod = new(time.Duration)
	fs.DurationVar(option.SyncPeriod, "sync-period", time.Minute*10, "determines the minimum frequency at which watched resources are reconciled.")
}
