/*
Copyright 2022 The KubeVela Authors.

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

package watcher

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

type applicationMetricsWatcher struct {
	mu               sync.Mutex
	phaseCounter     map[string]int
	stepPhaseCounter map[string]int
	phaseDirty       map[string]struct{}
	stepPhaseDirty   map[string]struct{}
	informer         ctrlcache.Informer
	stopCh           chan struct{}
}

func (watcher *applicationMetricsWatcher) getApp(obj interface{}) *v1beta1.Application {
	app := &v1beta1.Application{}
	bs, _ := json.Marshal(obj)
	_ = json.Unmarshal(bs, app)
	return app
}

func (watcher *applicationMetricsWatcher) getPhase(phase string) string {
	if phase == "" {
		return "-"
	}
	return phase
}

func (watcher *applicationMetricsWatcher) inc(app *v1beta1.Application, delta int) {
	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	phase := watcher.getPhase(string(app.Status.Phase))
	watcher.phaseCounter[phase] += delta
	watcher.phaseDirty[phase] = struct{}{}
	if app.Status.Workflow != nil {
		for _, step := range app.Status.Workflow.Steps {
			stepPhase := watcher.getPhase(string(step.Phase))
			key := fmt.Sprintf("%s/%s#%s", step.Type, stepPhase, step.Reason)
			watcher.stepPhaseCounter[key] += delta
			watcher.stepPhaseDirty[key] = struct{}{}
		}
	}
}

func (watcher *applicationMetricsWatcher) report() {
	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	for phase := range watcher.phaseDirty {
		metrics.ApplicationPhaseCounter.WithLabelValues(phase).Set(float64(watcher.phaseCounter[phase]))
	}
	for stepPhase := range watcher.stepPhaseDirty {
		metrics.WorkflowStepPhaseGauge.WithLabelValues(strings.Split(stepPhase, "/")[:2]...).Set(float64(watcher.stepPhaseCounter[stepPhase]))
	}
	watcher.phaseDirty = map[string]struct{}{}
	watcher.stepPhaseDirty = map[string]struct{}{}
}

func (watcher *applicationMetricsWatcher) run() {
	go func() {
		for {
			select {
			case <-watcher.stopCh:
				return
			default:
				time.Sleep(time.Second)
				watcher.report()
			}
		}
	}()
}

// StartApplicationMetricsWatcher start metrics watcher for reporting application stats
func StartApplicationMetricsWatcher(informer ctrlcache.Informer) {
	watcher := &applicationMetricsWatcher{
		phaseCounter:     map[string]int{},
		stepPhaseCounter: map[string]int{},
		phaseDirty:       map[string]struct{}{},
		stepPhaseDirty:   map[string]struct{}{},
		informer:         informer,
		stopCh:           make(chan struct{}, 1),
	}
	_, err := watcher.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			app := watcher.getApp(obj)
			watcher.inc(app, 1)
		},
		UpdateFunc: func(oldObj, obj interface{}) {
			oldApp := watcher.getApp(oldObj)
			app := watcher.getApp(obj)
			watcher.inc(oldApp, -1)
			watcher.inc(app, 1)
		},
		DeleteFunc: func(obj interface{}) {
			app := watcher.getApp(obj)
			watcher.inc(app, -1)
		},
	})
	if err != nil {
		klog.ErrorS(err, "failed to add event handler for application metrics watcher")
	}
	watcher.run()
}
