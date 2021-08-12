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
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var (
	PerfEnabled = os.Getenv("PERF") == "1"
)

const (
	reconcileTimeout = time.Minute
)

type ReconcileEvent struct {
	Name string
	Time time.Time
}

type ReconcileContext struct {
	context.Context
	logr.Logger
	controllerName string
	req ctrl.Request
	cancel context.CancelFunc
	begin time.Time
	events []*ReconcileEvent
}

func getControllerName() string {
	if _, file, _, ok := runtime.Caller(2); ok {
		s := filepath.Base(file)
		if strings.HasSuffix(s, ".go") {
			return strings.TrimSuffix(s, ".go")
		} else {
			return s
		}
	}
	return "-"
}

func NewReconcileContext(ctx context.Context, req ctrl.Request) *ReconcileContext {
	controllerName := getControllerName()
	logger := klogr.New().WithValues("namespace", req.Namespace, "name", req.Name, "controller", controllerName)
	ctx = util.SetNamespaceInCtx(ctx, req.Namespace)
	ctx, cancel := context.WithTimeout(ctx, reconcileTimeout)
	return &ReconcileContext{
		Context: ctx,
		Logger: logger,
		controllerName: controllerName,
		req: req,
		cancel: cancel,
		events: []*ReconcileEvent{},
	}
}

func (ctx *ReconcileContext) BeginReconcile() {
	ctx.begin = time.Now()
	logr.WithCallDepth(ctx.Logger, 1).Info("Begin reconcile")
}

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
	}
}

func (ctx *ReconcileContext) AddEvent(name string) {
	if PerfEnabled {
		ctx.events = append(ctx.events, &ReconcileEvent{
			Name: name,
			Time: time.Now(),
		})
	}
}