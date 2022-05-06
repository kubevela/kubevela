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

package main

import (
	"k8s.io/klog/v2"
	"sigs.k8s.io/apiserver-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela/apis/apiextensions.core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/apiextensions/applicationresourcetracker"
)

func main() {
	cfg := config.GetConfigOrDie()
	cmd, err := builder.APIServer.
		WithResourceAndHandler(
			&v1alpha1.ApplicationResourceTracker{},
			applicationresourcetracker.NewResourceHandlerProvider(cfg),
		).
		WithLocalDebugExtension().
		ExposeLoopbackMasterClientConfig().
		ExposeLoopbackAuthorizer().
		WithoutEtcd().
		Build()
	if err != nil {
		klog.Fatal(err)
	}

	if err = cmd.Execute(); err != nil {
		klog.Fatal(err)
	}
}
