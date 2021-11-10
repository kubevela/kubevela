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

package controllers_test

import (
	"context"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var verifyComponentCreated = func(testcase, namespace, compName string) {

	Eventually(
		func() error {
			comp := v1alpha2.Component{}
			return k8sClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: compName}, &comp)
		},
		time.Second*3, 30*time.Millisecond).Should(BeNil(), "check component created fail for test "+testcase)
}
