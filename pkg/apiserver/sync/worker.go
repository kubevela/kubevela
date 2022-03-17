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

package sync

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
)

// Start prepares watchers and run their controllers, then waits for process termination signals
func Start(ctx context.Context, ds datastore.DataStore, cfg *rest.Config) {
	k8sClient, err := clients.GetKubeClient()
	if err != nil {
		logrus.Fatal(err)
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		logrus.Fatal(err)
	}

	f := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 0, v1.NamespaceAll, nil)

	startAppSyncing(ctx, f, ds, k8sClient)
}

func startAppSyncing(ctx context.Context, factory dynamicinformer.DynamicSharedInformerFactory, ds datastore.DataStore, cli client.Client) {
	var err error
	informer := factory.ForResource(v1beta1.SchemeGroupVersion.WithResource("applications")).Informer()
	getApp := func(obj interface{}) *v1beta1.Application {
		app := &v1beta1.Application{}
		bs, _ := json.Marshal(obj)
		_ = json.Unmarshal(bs, app)
		return app
	}
	cu := &CR2UX{
		ds:    ds,
		cli:   cli,
		cache: sync.Map{},
	}
	if err = cu.InitCache(ctx); err != nil {
		klog.Fatal("sync app init err", err)
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			app := getApp(obj)
			klog.Infof("watched add app event, namespace: %s, name: %s", app.Namespace, app.Name)
			err = cu.AddOrUpdate(ctx, app)
			if err != nil {
				logrus.Errorf("Application %-30s Create Sync to db err %v", color.WhiteString(app.Namespace+"/"+app.Name), err)
			}
		},
		UpdateFunc: func(oldObj, obj interface{}) {
			app := getApp(obj)
			klog.Infof("watched update app event, namespace: %s, name: %s", app.Namespace, app.Name)
			err = cu.AddOrUpdate(ctx, app)
			if err != nil {
				klog.Errorf("Application %-30s Update Sync to db err %v", color.WhiteString(app.Namespace+"/"+app.Name), err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			app := getApp(obj)
			klog.Infof("watched delete app event, namespace: %s, name: %s", app.Namespace, app.Name)
			err = cu.DeleteApp(ctx, app)
			if err != nil {
				klog.Errorf("Application %-30s Deleted Sync to db err %v", color.WhiteString(app.Namespace+"/"+app.Name), err)
			}
		},
	}
	informer.AddEventHandler(handlers)
	klog.Info("app syncing started")
	informer.Run(ctx.Done())
}
