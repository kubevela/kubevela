package server

import (
	"context"
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/cloud-native-application/rudrx/pkg/server"
)

func main() {
	var logFilePath string
	var logRetainDate int
	var logCompress, development bool

	flag.StringVar(&logFilePath, "log-file-path", "", "The address the metric endpoint binds to.")
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
	server := server.ApiServer{}
	server.Launch()
	// handle signal: SIGTERM(15)
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGTERM)
	select {
	case <-sc:
		ctx, _ := context.WithTimeout(context.Background(), time.Minute)
		go server.Shutdown(ctx)
	}
}
