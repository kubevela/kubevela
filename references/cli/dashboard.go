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

package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/oam-dev/kubevela/pkg/utils/common"

	"github.com/mholt/archiver/v3"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/apiserver"
	"github.com/oam-dev/kubevela/references/apiserver/util"
)

// NewDashboardCommand creates `dashboard` command and its nested children commands
func NewDashboardCommand(c common.Args, ioStreams cmdutil.IOStreams, frontendSource string) *cobra.Command {
	var o Options
	o.frontendSource = frontendSource
	cmd := &cobra.Command{
		Hidden:     true,
		Use:        "dashboard",
		Short:      "Setup API Server and launch Dashboard",
		Long:       "Setup API Server and launch Dashboard",
		Example:    `dashboard`,
		Deprecated: "vela dashboard is deprecated, it will only launch APIServer without dashboard",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			if o.skipcheck {
				return SetupAPIServer(c, cmd, o)
			}
			runtimeReady, err := CheckVelaRuntimeInstalledAndReady(ioStreams, newClient)
			if err != nil {
				return err
			}
			if !runtimeReady {
				return nil
			}
			return SetupAPIServer(c, cmd, o)
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
	cmd.Flags().BoolVar(&o.skipcheck, "skip-check", false, "skip check vela core status and run directly")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// Options creates options for `dashboard` command
type Options struct {
	logFilePath    string
	logRetainDate  int
	logCompress    bool
	development    bool
	staticPath     string
	port           string
	frontendSource string
	skipcheck      bool
}

// GetStaticPath gets the path of front-end directory
func (o *Options) GetStaticPath() error {
	if o.frontendSource == "" {
		return nil
	}
	var err error
	o.staticPath, err = system.GetDefaultFrontendDir()
	if err != nil {
		return fmt.Errorf("get fontend dir err %w", err)
	}
	_ = os.RemoveAll(o.staticPath)
	//nolint:gosec
	err = os.MkdirAll(o.staticPath, 0755)
	if err != nil {
		return fmt.Errorf("create fontend dir err %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(o.frontendSource)
	if err != nil {
		return fmt.Errorf("decode frontendSource err %w", err)
	}
	tgzpath := filepath.Join(o.staticPath, "frontend.tgz")
	//nolint:gosec
	err = ioutil.WriteFile(tgzpath, data, 0644)
	if err != nil {
		return fmt.Errorf("write frontend.tgz to static path err %w", err)
	}
	//nolint:errcheck
	defer os.Remove(tgzpath)
	tgz := archiver.NewTarGz()
	//nolint:errcheck
	defer tgz.Close()
	if err = tgz.Unarchive(tgzpath, o.staticPath); err != nil {
		return fmt.Errorf("write static files to fontend dir err %w", err)
	}
	files, err := ioutil.ReadDir(o.staticPath)
	if err != nil {
		return fmt.Errorf("read static file %s err %w", o.staticPath, err)
	}
	var name string
	for _, fi := range files {
		if fi.IsDir() {
			name = fi.Name()
			break
		}
	}
	if name == "" {
		return fmt.Errorf("no static dir found in %s", o.staticPath)
	}
	o.staticPath = filepath.Join(o.staticPath, name)
	return nil
}

// SetupAPIServer starts a RESTfulAPI server
func SetupAPIServer(c common.Args, cmd *cobra.Command, o Options) error {
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
		if err = o.GetStaticPath(); err != nil {
			cmd.Printf("Get static file error %v, will only serve as Restful API", err)
		}
	}

	if !strings.HasPrefix(o.port, ":") {
		o.port = ":" + o.port
	}

	// Setup RESTful server
	server, err := apiserver.New(c, o.port, o.staticPath)
	if err != nil {
		return err
	}
	errCh := make(chan error, 1)
	cmd.Printf("Serving at %v\nstatic dir is %v", o.port, o.staticPath)

	server.Launch(errCh)
	select {
	case err = <-errCh:
		return err
	case <-time.After(time.Second):
		var url = "http://127.0.0.1" + o.port
		if o.staticPath != "" {
			if err := OpenBrowser(url); err != nil {
				cmd.Printf("Invoke browser err %v\nPlease Visit %s to see dashboard", err, url)
			}
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

// CheckVelaRuntimeInstalledAndReady checks whether vela-core runtime is installed and ready
func CheckVelaRuntimeInstalledAndReady(ioStreams cmdutil.IOStreams, c client.Client) (bool, error) {
	if !helm.IsHelmReleaseRunning(types.DefaultKubeVelaReleaseName, types.DefaultKubeVelaChartName, types.DefaultKubeVelaNS, ioStreams) {
		ioStreams.Info(fmt.Sprintf("\n%s %s", emojiFail, "KubeVela runtime is not installed yet."))
		ioStreams.Info(fmt.Sprintf("\n%s %s%s",
			emojiLightBulb,
			"Please use this command to install: ",
			white.Sprint("helm repo add kubevela https://kubevelacharts.oss-cn-hangzhou.aliyuncs.com/core && "+
				"helm repo update \n kubectl create namespace vela-system \n "+
				"helm install -n vela-system kubevela kubevela/vela-core"),
		))
		return false, nil
	}
	return PrintTrackVelaRuntimeStatus(context.Background(), c, ioStreams, 5*time.Minute)
}
