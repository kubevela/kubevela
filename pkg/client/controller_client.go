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

package client

import (
	"context"
	"strings"
	"sync"

	"github.com/oam-dev/kubevela/pkg/resourcetracker"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	cache2 "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

var (
	// CachedGVKs identifies the GVKs of resources to be cached during dispatching
	CachedGVKs = ""

	rtCount = 0
	lock    = sync.Mutex{}
)

// DefaultNewControllerClient function for creating controller client
func DefaultNewControllerClient(cache cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (c client.Client, err error) {
	rawClient, err := client.New(config, options)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get raw client")
	}

	mClient := &monitorClient{rawClient}
	if err := resourcetracker.AddResourceTrackerCacheIndex(cache); err != nil {
		return nil, errors.Wrapf(err, "failed to add app index to ResourceTracker cache")
	}
	mCache := &monitorCache{cache}

	uncachedStructuredGVKs := map[schema.GroupVersionKind]struct{}{}
	for _, obj := range uncachedObjects {
		gvk, err := apiutil.GVKForObject(obj, mClient.Scheme())
		if err != nil {
			return nil, err
		}
		uncachedStructuredGVKs[gvk] = struct{}{}
	}

	cachedUnstructuredGVKs := map[schema.GroupVersionKind]struct{}{}
	for _, s := range strings.Split(CachedGVKs, ",") {
		s = strings.Trim(s, " ")
		if len(s) > 0 {
			gvk, _ := schema.ParseKindArg(s)
			if gvk == nil {
				return nil, errors.Errorf("invalid cached gvk: %s", s)
			}
			cachedUnstructuredGVKs[*gvk] = struct{}{}
		}
	}

	informer, err := cache.GetInformerForKind(context.Background(), v1beta1.ResourceTrackerKindVersionKind)
	if err != nil {
		return nil, err
	}

	informer.AddEventHandler(cache2.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			lock.Lock()
			rtCount++
			metrics.ResourceTrackerNumberGauge.WithLabelValues("application").Set(float64(rtCount))
			lock.Unlock()
		},
		DeleteFunc: func(obj interface{}) {
			lock.Lock()
			rtCount--
			metrics.ResourceTrackerNumberGauge.WithLabelValues("application").Set(float64(rtCount))
			lock.Unlock()
		},
	})

	dClient := &delegatingClient{
		scheme: mClient.Scheme(),
		mapper: mClient.RESTMapper(),
		Reader: &delegatingReader{
			CacheReader:            mCache,
			ClientReader:           mClient,
			scheme:                 mClient.Scheme(),
			uncachedStructuredGVKs: uncachedStructuredGVKs,
			cachedUnstructuredGVKs: cachedUnstructuredGVKs,
		},
		Writer:       mClient,
		StatusClient: mClient,
	}

	return dClient, nil
}
