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

package hooks_test

import (
	"context"
	"testing"

	"github.com/kubevela/pkg/util/k8s"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/kubevela/pkg/util/test/bootstrap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/cmd/core/app/hooks"
	"github.com/oam-dev/kubevela/pkg/features"
)

var _ = bootstrap.InitKubeBuilderForTest(bootstrap.WithCRDPath("./testdata"))

func TestPreStartHook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run pre-start hook test")
}

var _ = Describe("Test pre-start hooks", func() {
	It("Test SystemCRDValidationHook", func() {
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, features.ZstdApplicationRevision, true)()
		ctx := context.Background()
		Expect(k8s.EnsureNamespace(ctx, singleton.KubeClient.Get(), types.DefaultKubeVelaNS)).Should(Succeed())
		err := hooks.NewSystemCRDValidationHook().Run(ctx)
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("the ApplicationRevision CRD is not updated"))
	})
})
