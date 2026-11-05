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

package cuex

import (
	"context"
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCompilerGuardSpecs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vela-core CueX compilers")
}

const metadataCall = "import \"vela/http\"\nr: http.#Do & {$params: {method: \"GET\", url: \"http://169.254.169.254/latest/meta-data/\"}}\n"

var _ = Describe("vela/http in every vela-core compiler", func() {
	It("is guarded in the workload compiler (components and traits)", func() {
		_, err := WorkloadCompiler.Get().CompileString(context.Background(), metadataCall)
		Expect(err).To(MatchError(ContainSubstring("blocked SSRF target")))
	})

	It("is guarded in the upstream default compiler after GuardDefaultCompiler (policies, webhook)", func() {
		previous := cuex.EnableExternalPackageForDefaultCompiler
		DeferCleanup(func() { cuex.EnableExternalPackageForDefaultCompiler = previous })
		cuex.EnableExternalPackageForDefaultCompiler = false

		GuardDefaultCompiler()
		_, err := cuex.DefaultCompiler.Get().CompileString(context.Background(), metadataCall)
		Expect(err).To(MatchError(ContainSubstring("blocked SSRF target")))
	})
})
