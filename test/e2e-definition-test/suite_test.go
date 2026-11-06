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

package definitions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	framework "github.com/oam-dev/kubevela/test/e2e-framework"
)

var support = framework.New()
var k8sClient client.Client
var scheme = support.Scheme
var _ = support.Register(false, func(cli client.Client) { k8sClient = cli })

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Definition E2E Suite")
}

var randomNamespaceName = framework.RandomNamespaceName
var testDataPath = framework.TestDataPath
var verifyApplicationPhase = support.VerifyApplicationPhase
var RequestReconcileNow = support.RequestReconcileNow
var scalerTrait = framework.ScalerTrait
var scalerTraitOutputTemplate = framework.ScalerTraitOutputTemplate
