package cmd

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/cmd/dashboard"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/server"
	"github.com/cloud-native-application/rudrx/pkg/server/util"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func NewDashboardCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var o Options
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
			return SetupAPIServer(newClient, cmd, o)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.Flags().StringVar(&o.logFilePath, "log-file-path", "", "The log file path.")
	cmd.Flags().IntVar(&o.logRetainDate, "log-retain-date", 7, "The number of days of logs history to retain.")
	cmd.Flags().BoolVar(&o.logCompress, "log-compress", true, "Enable compression on the rotated logs.")
	cmd.Flags().BoolVar(&o.development, "development", true, "Development mode.")
	cmd.Flags().StringVar(&o.staticPath, "static", "", "specify local static file directory")
	cmd.Flags().StringVar(&o.port, "port", util.DefaultDashboardPort, "specify port for dashboard")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

type Options struct {
	logFilePath   string
	logRetainDate int
	logCompress   bool
	development   bool
	staticPath    string
	port          string
}

func SetupAPIServer(kubeClient client.Client, cmd *cobra.Command, o Options) error {

	// setup logging
	var w io.Writer
	if len(o.logFilePath) > 0 {
		w = zapcore.AddSync(&lumberjack.Logger{
			Filename: o.logFilePath,
			MaxAge:   o.logRetainDate, // days
			Compress: o.logCompress,
		})
	} else {
		w = os.Stdout
	}
	ctrl.SetLogger(zap.New(func(zo *zap.Options) {
		zo.Development = o.development
		zo.DestWritter = w
	}))

	var err error
	if o.staticPath == "" {
		o.staticPath, err = system.GetDefaultFrontendDir()
		if err != nil {
			return fmt.Errorf("get fontend dir err %v", err)
		}
		_ = os.RemoveAll(o.staticPath)
		err = os.MkdirAll(o.staticPath, 0755)
		if err != nil {
			return fmt.Errorf("create fontend dir err %v", err)
		}
		if err = ioutil.WriteFile(filepath.Join(o.staticPath, "index.html"), []byte(dashboard.IndexHTML), 0644); err != nil {
			return fmt.Errorf("write index.html to fontend dir err %v", err)
		}
	}

	if !strings.HasPrefix(o.port, ":") {
		o.port = ":" + o.port
	}

	//Setup RESTful server
	server := server.APIServer{}

	errCh := make(chan error, 1)

	server.Launch(kubeClient, o.port, o.staticPath, errCh)
	select {
	case err = <-errCh:
		return err
	case <-time.After(time.Second):
		var url = "http://127.0.0.1" + o.port
		if err := OpenBrowser(url); err != nil {
			cmd.Printf("Invoke browser err %v\nPlease Visit %s to see dashboard", err, url)
		}
	}

	// handle signal: SIGTERM(15)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGTERM)

	<-sc
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func OpenBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("cmd", "/C", "start", url).Run()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}
