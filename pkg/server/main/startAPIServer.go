package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/server"
	"github.com/oam-dev/kubevela/pkg/server/util"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// main will only start up API server
func main() {
	var development = true
	// setup logging
	var w io.Writer = os.Stdout

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = development
		o.DestWritter = w
	}))

	c, err := oam.InitArgs()
	if err != nil {
		ctrl.Log.Error(err, "failed to init Kubernetes Config")
		os.Exit(1)
	}
	apiServer, err := server.New(c, util.DefaultAPIServerPort, "")
	if err != nil {
		ctrl.Log.Error(err, "failed to init dashboard server")
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	apiServer.Launch(errCh)
	err = <-errCh
	if err != nil {
		ctrl.Log.Error(err, "failed to launch API server")
	}
	// handle signal: SIGTERM(15)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGTERM)
	<-sc
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		ctrl.Log.Error(err, "failed to shut down API server")
	}

}
