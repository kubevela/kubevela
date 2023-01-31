/*
Copyright 2023 The KubeVela Authors.

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

package sharding

import (
	"context"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/maps"
	velaruntime "github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/podutils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// Scheduler schedule shard-id for object
type Scheduler interface {
	Start(context.Context)
	Schedule(client.Object) bool
}

var _ Scheduler = (*staticScheduler)(nil)

// NewStaticScheduler create a scheduler that do not make update but only use predefined shards for allocate
func NewStaticScheduler(shards []string) Scheduler {
	return &staticScheduler{shards: shards}
}

type staticScheduler struct {
	shards []string
}

// Start .
func (in *staticScheduler) Start(ctx context.Context) {
	klog.Infof("staticScheduler started, shards: [%s]", strings.Join(in.shards, ", "))
}

// Schedule the target object to a random shard
func (in *staticScheduler) Schedule(o client.Object) bool {
	if _, scheduled := GetScheduledShardID(o); !scheduled {
		if len(in.shards) > 0 {
			// nolint
			sid := in.shards[rand.Intn(len(in.shards))]
			klog.Infof("staticScheduler schedule %s %s/%s to shard[%s]", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName(), sid)
			SetScheduledShardID(o, sid)
			return true
		}
		klog.Infof("staticDiscoveryScheduler no schedulable shard found for %s %s/%s", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName())
	}
	return false
}

var _ Scheduler = (*dynamicDiscoveryScheduler)(nil)

// NewDynamicDiscoveryScheduler create a scheduler that allow dynamic discovery for available shards
func NewDynamicDiscoveryScheduler(name string, resyncPeriod time.Duration) Scheduler {
	return &dynamicDiscoveryScheduler{
		name:         name,
		resyncPeriod: resyncPeriod,
		candidates:   map[string]map[string]bool{},
	}
}

type dynamicDiscoveryScheduler struct {
	mu sync.RWMutex

	name            string
	resyncPeriod    time.Duration
	candidates      map[string]map[string]bool
	roundRobinIndex atomic.Uint32

	store    cache.Store
	informer cache.Controller
}

func (in *dynamicDiscoveryScheduler) _registerPod(obj interface{}) {
	if pod, ok := obj.(*corev1.Pod); ok {
		id := pod.GetLabels()[LabelKubeVelaShardID]
		healthy := podutils.IsPodReady(pod)
		klog.Infof("dynamicDiscoveryScheduler register pod %s/%s (id: %s) with health status: %t", pod.Namespace, pod.Name, id, healthy)
		in.mu.Lock()
		defer in.mu.Unlock()
		if _, exist := in.candidates[id]; !exist {
			in.candidates[id] = map[string]bool{}
		}
		in.candidates[id][pod.Name] = healthy
	}
}

func (in *dynamicDiscoveryScheduler) _unregisterPod(obj interface{}) {
	if pod, ok := obj.(*corev1.Pod); ok {
		id := pod.GetLabels()[LabelKubeVelaShardID]
		klog.Infof("dynamicDiscoveryScheduler unregister pod %s/%s", pod.Namespace, pod.Name)
		in.mu.Lock()
		defer in.mu.Unlock()
		if _, exist := in.candidates[id]; exist {
			delete(in.candidates[id], pod.Name)
			if len(in.candidates[id]) == 0 {
				delete(in.candidates, id)
			}
		}
	}
}

// resync the available shards
func (in *dynamicDiscoveryScheduler) resync(stopCh <-chan struct{}) {
	ticker := time.NewTicker(in.resyncPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			in.mu.Lock()
			in.candidates = map[string]map[string]bool{}
			in.mu.Unlock()
			for _, obj := range in.store.List() {
				in._registerPod(obj)
			}
			available := in.availableShards()
			klog.Infof("dynamicDiscoveryScheduler resync finished, available shards: [%s]", strings.Join(available, ", "))
		}
	}
}

// Start run scheduler to watch pods and automatic register
func (in *dynamicDiscoveryScheduler) Start(ctx context.Context) {
	klog.Infof("dynamicDiscoveryScheduler staring, watching pods in %s", util.GetRuntimeNamespace())
	cli := singleton.StaticClient.Get().CoreV1().RESTClient()
	lw := cache.NewFilteredListWatchFromClient(cli, "pods", util.GetRuntimeNamespace(), func(options *metav1.ListOptions) {
		ls := labels.NewSelector()
		ls = ls.Add(*velaruntime.Must(labels.NewRequirement(LabelKubeVelaShardID, selection.Exists, nil)))
		ls = ls.Add(*velaruntime.Must(labels.NewRequirement("app.kubernetes.io/name", selection.Equals, []string{in.name})))
		options.LabelSelector = ls.String()
	})
	in.store, in.informer = cache.NewInformer(lw, &corev1.Pod{}, in.resyncPeriod, cache.ResourceEventHandlerFuncs{
		AddFunc: in._registerPod,
		UpdateFunc: func(oldObj, newObj interface{}) {
			if k8s.GetLabel(oldObj.(runtime.Object), LabelKubeVelaShardID) != k8s.GetLabel(newObj.(runtime.Object), LabelKubeVelaShardID) {
				in._unregisterPod(oldObj)
			}
			in._registerPod(newObj)
		},
		DeleteFunc: in._unregisterPod,
	})
	stopCh := ctx.Done()
	if stopCh == nil {
		stopCh = make(chan struct{})
	}
	if in.resyncPeriod > 0 {
		go in.resync(stopCh)
	}
	klog.Infof("dynamicDiscoveryScheduler started")
	in.informer.Run(stopCh)
}

func (in *dynamicDiscoveryScheduler) availableShards() []string {
	in.mu.RLock()
	defer in.mu.RUnlock()
	var available []string
	for id, pods := range in.candidates {
		if slices.Any(maps.Values(pods), func(x bool) bool { return x }) {
			available = append(available, id)
		}
	}
	return available
}

func (in *dynamicDiscoveryScheduler) schedule() (string, bool) {
	available := in.availableShards()
	if len(available) == 0 {
		return "", false
	}
	sort.Strings(available)
	idx := in.roundRobinIndex.Add(1) % uint32(len(available))
	return available[idx], true
}

// Schedule get available shard-id for application
func (in *dynamicDiscoveryScheduler) Schedule(o client.Object) bool {
	if _, scheduled := GetScheduledShardID(o); !scheduled {
		if sid, ok := in.schedule(); ok {
			klog.Infof("dynamicDiscoveryScheduler schedule %s %s/%s to shard[%s]", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName(), sid)
			SetScheduledShardID(o, sid)
			return true
		}
		klog.Infof("dynamicDiscoveryScheduler no schedulable shard found for %s %s/%s", o.GetObjectKind().GroupVersionKind().Kind, o.GetNamespace(), o.GetName())
	}
	return false
}
