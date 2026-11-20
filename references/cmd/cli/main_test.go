/*
Copyright 2026 The KubeVela Authors.

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
	"testing"

	workflowfeatures "github.com/kubevela/workflow/pkg/features"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/features"
)

func TestCLIMain(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vela CLI main")
}

var _ = Describe("CLI feature gates", func() {
	BeforeEach(func() {
		Expect(setCLIFeatureGates()).To(Succeed())
	})

	It("still turns on the other alpha gates", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Enabled(features.DeprecatedPolicySpec)).To(BeTrue())
	})

	It("keeps the workflow HTTP guard gates off", func() {
		Expect(utilfeature.DefaultMutableFeatureGate.Enabled(workflowfeatures.DisableWorkflowHTTP)).To(BeFalse())
		Expect(utilfeature.DefaultMutableFeatureGate.Enabled(workflowfeatures.BlockPrivateHTTPAddresses)).To(BeFalse())
	})
})
