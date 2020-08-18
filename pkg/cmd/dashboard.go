package cmd

import (
	"context"
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/cloud-native-application/rudrx/api/types"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/server"
	"github.com/spf13/cobra"
)

func NewDashboardCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	// ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "dashboard",
		Short:   "Setup API Server and launch Dashboard",
		Long:    "Setup API Server and launch Dashboard",
		Example: `dashboard`,
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			SetupApiServer(newClient)
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

/*
SetupApiServer moved main function from cmd/server/main.go written by Ryan, so it can be integrated
by Cli and called by cmd/server/main.go.
*/
func SetupApiServer(kubeClient client.Client) {
	var logFilePath string
	var logRetainDate int
	var logCompress, development bool

	flag.StringVar(&logFilePath, "log-file-path", "", "The log file path.")
	flag.IntVar(&logRetainDate, "log-retain-date", 7, "The number of days of logs history to retain.")
	flag.BoolVar(&logCompress, "log-compress", true, "Enable compression on the rotated logs.")
	flag.BoolVar(&development, "development", true, "Development mode.")
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
		o.Development = development
		o.DestWritter = w
	}))

	//Setup RESTful server
	server := server.ApiServer{}
	server.Launch(kubeClient)
	// handle signal: SIGTERM(15), SIGKILL(9)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGTERM)
	signal.Notify(sc, syscall.SIGKILL)
	select {
	case <-sc:
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		server.Shutdown(ctx)
	}
}
