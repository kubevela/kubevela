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
	"os"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/oam"
)

var (
	singletonConfig        *rest.Config
	singletonDynamicClient client.Client
	singletonClient        *kubernetes.Clientset
)

// Config returns a Kubernetes config
func Config() *rest.Config {
	if singletonConfig != nil {
		return singletonConfig
	}
	var err error
	singletonConfig, err = config.GetConfig()
	singletonConfig.Wrap(pkgmulticluster.NewTransportWrapper())
	if err != nil {
		klog.Error(err, "Fail to get Kubernetes config")
		os.Exit(1)
	}
	return singletonConfig
}

// DynamicClient will return Kubernetes client from controller-runtime package
func DynamicClient() client.Client {
	if singletonDynamicClient != nil {
		return singletonDynamicClient
	}

	c := Config()
	var err error
	singletonDynamicClient, err = client.New(c, client.Options{Scheme: Scheme})
	if err != nil {
		klog.Error(err, "Fail to create Kubernetes dynamic client")
		os.Exit(1)
	}
	return singletonDynamicClient
}

// GetFakeClient will return a fake client contains some pre-defined objects
func GetFakeClient(defs []oam.Object) client.Client {
	objs := make([]client.Object, 0, len(defs))
	for _, def := range defs {
		if unstructDef, ok := def.(*unstructured.Unstructured); ok {
			objs = append(objs, unstructDef)
		}
	}
	return fake.NewClientBuilder().WithObjects(objs...).WithScheme(Scheme).Build()
}

// Client returns a Kubernetes client from client-go package
func Client() *kubernetes.Clientset {
	if singletonClient != nil {
		return singletonClient
	}

	c := Config()
	var err error
	singletonClient, err = kubernetes.NewForConfig(c)
	if err != nil {
		klog.Error(err, "Fail to create Kubernetes client")
		os.Exit(1)
	}
	return singletonClient
}

// SetConfig will set the given config to singleton config
func SetConfig(c *rest.Config) {
	if c != nil {
		singletonConfig = c
		reloadClient()
	}
}

// SetClient will set the given client to singleton client
func SetClient(c client.Client) {
	singletonDynamicClient = c
}

func reloadClient() {
	_ = Client()
	_ = DynamicClient()
}
