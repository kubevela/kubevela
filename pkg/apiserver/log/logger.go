package log

import (
	"go.uber.org/zap"
)

// Logger Log component
var Logger *zap.SugaredLogger

func init() {
	l, _ := zap.NewProduction()
	Logger = l.Sugar()
}
