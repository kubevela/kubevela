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

package cli

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestGetKubeVelaHelmChartRepoURL(t *testing.T) {
	assert.Equal(t, getKubeVelaHelmChartRepoURL("v1.2.2"), "https://charts.kubevela.net/core/vela-core-1.2.2.tgz")
	assert.Equal(t, getKubeVelaHelmChartRepoURL("1.1.11"), "https://charts.kubevela.net/core/vela-core-1.1.11.tgz")
}

var _ = Describe("Test Install Command", func() {
	It("Test checkKubeServerVersion", func() {
		new, _, err := checkKubeServerVersion(cfg)
		Expect(err).Should(BeNil())
		Expect(new).Should(BeFalse())
	})
})
