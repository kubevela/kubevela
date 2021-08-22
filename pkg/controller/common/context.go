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

package common

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/klogr"

	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var (
	// PerfEnabled identify whether to add performance log for controllers
	PerfEnabled = os.Getenv("PERF") == "1"
)

var (
	// PerfEnabled identify whether to add performance log for controllers
	PerfEnabled = false
)

const (
	reconcileTimeout = time.Minute
)

// ReconcileEvent record the event name and time that it happened
type ReconcileEvent struct {
	Name string
	Time time.Time
}

// ReconcileContext keeps the context of one reconcile
type ReconcileContext struct {
	context.Context
	logr.Logger
	callerName string
	obj        types.NamespacedName
	cancel     context.CancelFunc
	begin      time.Time
	events     []*ReconcileEvent
	timestamps map[string]time.Time
	timers     map[string]time.Duration
}

func getCallerName() string {
	if _, file, _, ok := runtime.Caller(2); ok {
		s := filepath.Base(file)
		if strings.Contains(file, "handler") {
			s = filepath.Base(filepath.Dir(file)) + "_" + s
		}
		if strings.HasSuffix(s, ".go") {
			return strings.TrimSuffix(s, ".go")
		}
		return s
	}
	return "-"
}

// NewReconcileContext create new context for one reconcile
func NewReconcileContext(ctx context.Context, obj types.NamespacedName) *ReconcileContext {
	callerName := getCallerName()
	logger := klogr.New().WithValues("namespace", obj.Namespace, "name", obj.Name, "caller", callerName)
	ctx = util.SetNamespaceInCtx(ctx, obj.Namespace)
	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	return &ReconcileContext{
		Context:    ctx,
		Logger:     logger,
		callerName: callerName,
		obj:        obj,
		cancel:     cancel,
		events:     []*ReconcileEvent{},
		timestamps: map[string]time.Time{},
		timers:     map[string]time.Duration{},
	}
}

// BeginReconcile starts recording for new reconcile
func (ctx *ReconcileContext) BeginReconcile() {
	ctx.begin = time.Now()
	logr.WithCallDepth(ctx.Logger, 1).Info("Begin reconcile")
}

// EndReconcile ends recording for current reconcile and print out all performance checkpoints if PerfEnabled
func (ctx *ReconcileContext) EndReconcile() {
	ctx.cancel()
	t0 := time.Now()
	logger := logr.WithCallDepth(ctx.Logger, 1)
	logger.Info("End reconcile", "elapsed", t0.Sub(ctx.begin))
	t := ctx.begin
	if PerfEnabled {
		for _, event := range ctx.events {
			logger.Info("Performance", "event", event.Name, "elapsed", event.Time.Sub(t))
			t = event.Time
		}
		logger.Info("Performance", "event", "end_reconcile", "elapsed", time.Since(t0))
		for name, _t := range ctx.timers {
			logger.Info("Performance", "event", "timer::"+name, "elapsed", _t)
		}
	}
}

// AddEvent add event checkpoint during reconcile. The recorded event time cost will be recorded when calling EndReconcile
func (ctx *ReconcileContext) AddEvent(name string) {
	if PerfEnabled {
		ctx.events = append(ctx.events, &ReconcileEvent{
			Name: name,
			Time: time.Now(),
		})
	}
}

// BeginTimer add new timestamps
func (ctx *ReconcileContext) BeginTimer(name string) {
	ctx.timestamps[name] = time.Now()
}

// EndTimer calculate time cost since target name timestamp
func (ctx *ReconcileContext) EndTimer(name string) {
	if t, ok := ctx.timestamps[name]; ok {
		if _, _ok := ctx.timers[name]; !_ok {
			ctx.timers[name] = time.Duration(0)
		}
		ctx.timers[name] += time.Since(t)
	}
}

// GetLogger get logger from ReconcileContext
func (ctx *ReconcileContext) GetLogger() (logr.Logger, error) {
	if ctx.Logger == nil {
		return nil, errors.New("logger is not ready")
	}
	return ctx.Logger, nil
}
