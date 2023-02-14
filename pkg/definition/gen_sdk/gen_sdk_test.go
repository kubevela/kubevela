package gen_sdk

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test Generating SDK", func() {
	var err error
	outputDir := filepath.Join("testdata", "output")
	meta := GenMeta{
		Output:  outputDir,
		Lang:    "go",
		Package: "github.com/kubevela/test-gen-sdk",
		File:    []string{filepath.Join("testdata", "cron-task.cue")},
		InitSDK: true,
	}
	checkDirNotEmpty := func(dir string) {
		_, err = os.Stat(dir)
		Expect(err).Should(BeNil())
	}
	genWithMeta := func(meta GenMeta) {
		err = meta.Init(common.Args{})
		Expect(err).Should(BeNil())
		err = meta.CreateScaffold()
		Expect(err).Should(BeNil())
		err = meta.PrepareGeneratorAndTemplate()
		Expect(err).Should(BeNil())
		err = meta.Run()
		Expect(err).Should(BeNil())
	}
	It("Test generating SDK and init the scaffold", func() {
		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis"))
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "component", "cron-task"))
	})

	It("Test generating SDK, append apis", func() {
		meta.InitSDK = false
		meta.File = append(meta.File, "testdata/shared-resource.cue")

		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "policy", "shared-resource"))
	})

	It("Test free form parameter {...}", func() {
		meta.InitSDK = false
		meta.File = []string{"testdata/json-merge-patch.cue"}
		meta.Verbose = true

		genWithMeta(meta)
		checkDirNotEmpty(filepath.Join(outputDir, "pkg", "apis", "trait", "json-merge-patch"))
	})

	It("Test known issue: apply-terraform-provider", func() {
		meta.InitSDK = false
		meta.Verbose = true
		meta.File = []string{"testdata/apply-terraform-provider.cue"}
		genWithMeta(meta)
	})

	AfterSuite(func() {
		By("Cleaning up generated files")
		_ = os.RemoveAll(outputDir)
	})

})
