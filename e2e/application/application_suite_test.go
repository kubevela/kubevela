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

package e2e

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/e2e"
)

func TestApplication(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Application Suite")
}

// vela up takes its namespace from the current env and leaves it empty when
// none is set, which a fresh vela home does not have. Ginkgo also shuffles the
// top-level containers, so no spec can rely on another having run env sw.
var _ = ginkgo.BeforeSuite(func() {
	output, err := e2e.Exec("vela env sw default")
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), output)
})
