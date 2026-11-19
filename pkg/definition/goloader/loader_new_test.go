/*
Copyright 2025 The KubeVela Authors.

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

package goloader_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/goloader"
)

func TestGoloader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Goloader Suite")
}

var _ = Describe("Goloader", func() {
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "goloader-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("IsGoFile", func() {
		It("should return true for .go files", func() {
			Expect(goloader.IsGoFile("component.go")).To(BeTrue())
			Expect(goloader.IsGoFile("/path/to/component.go")).To(BeTrue())
			Expect(goloader.IsGoFile("my-definition.go")).To(BeTrue())
		})

		It("should return false for test files", func() {
			Expect(goloader.IsGoFile("component_test.go")).To(BeFalse())
			Expect(goloader.IsGoFile("/path/to/component_test.go")).To(BeFalse())
		})

		It("should return false for non-Go files", func() {
			Expect(goloader.IsGoFile("component.cue")).To(BeFalse())
			Expect(goloader.IsGoFile("component.yaml")).To(BeFalse())
			Expect(goloader.IsGoFile("component.json")).To(BeFalse())
			Expect(goloader.IsGoFile("README.md")).To(BeFalse())
		})
	})

	Describe("IsGoDefinitionFile", func() {
		It("should return true for files with defkit import", func() {
			goFile := filepath.Join(tempDir, "component.go")
			content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func MyComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("my-component")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			isDefFile, err := goloader.IsGoDefinitionFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDefFile).To(BeTrue())
		})

		It("should return false for files without defkit import", func() {
			goFile := filepath.Join(tempDir, "regular.go")
			content := `package main

import "fmt"

func main() {
	fmt.Println("Hello")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			isDefFile, err := goloader.IsGoDefinitionFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(isDefFile).To(BeFalse())
		})

		It("should return false for non-Go files", func() {
			isDefFile, err := goloader.IsGoDefinitionFile("component.cue")
			Expect(err).NotTo(HaveOccurred())
			Expect(isDefFile).To(BeFalse())
		})

		It("should return false for test files", func() {
			isDefFile, err := goloader.IsGoDefinitionFile("component_test.go")
			Expect(err).NotTo(HaveOccurred())
			Expect(isDefFile).To(BeFalse())
		})

		It("should return error for non-existent files", func() {
			_, err := goloader.IsGoDefinitionFile(filepath.Join(tempDir, "nonexistent.go"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("DiscoverDefinitions", func() {
		It("should find Go definition files in directory", func() {
			// Create definition file
			defFile := filepath.Join(tempDir, "component.go")
			defContent := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func MyComponent() *defkit.ComponentDefinition { return nil }
`
			err := os.WriteFile(defFile, []byte(defContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Create non-definition file
			regularFile := filepath.Join(tempDir, "util.go")
			regularContent := `package main
func helper() {}
`
			err = os.WriteFile(regularFile, []byte(regularContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			// Create test file (should be excluded)
			testFile := filepath.Join(tempDir, "component_test.go")
			testContent := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func TestComponent() {}
`
			err = os.WriteFile(testFile, []byte(testContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			files, err := goloader.DiscoverDefinitions(tempDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0]).To(Equal(defFile))
		})

		It("should find files in subdirectories", func() {
			subDir := filepath.Join(tempDir, "components")
			err := os.MkdirAll(subDir, 0755)
			Expect(err).NotTo(HaveOccurred())

			defFile := filepath.Join(subDir, "webservice.go")
			defContent := `package components
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func Webservice() *defkit.ComponentDefinition { return nil }
`
			err = os.WriteFile(defFile, []byte(defContent), 0600)
			Expect(err).NotTo(HaveOccurred())

			files, err := goloader.DiscoverDefinitions(tempDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(HaveLen(1))
			Expect(files[0]).To(Equal(defFile))
		})

		It("should return empty slice for directory with no definitions", func() {
			files, err := goloader.DiscoverDefinitions(tempDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(files).To(BeEmpty())
		})
	})

	Describe("AnalyzeGoFile", func() {
		It("should find ComponentDefinition functions", func() {
			goFile := filepath.Join(tempDir, "component.go")
			content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func WebserviceComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("webservice")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(HaveLen(1))
			Expect(defs[0].Name).To(Equal("webservice"))
			Expect(defs[0].Type).To(Equal("component"))
			Expect(defs[0].FunctionName).To(Equal("WebserviceComponent"))
			Expect(defs[0].PackageName).To(Equal("main"))
		})

		It("should find TraitDefinition functions", func() {
			goFile := filepath.Join(tempDir, "trait.go")
			content := `package traits

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func ScalerTrait() *defkit.TraitDefinition {
	return defkit.NewTrait("scaler")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(HaveLen(1))
			Expect(defs[0].Name).To(Equal("scaler"))
			Expect(defs[0].Type).To(Equal("trait"))
			Expect(defs[0].FunctionName).To(Equal("ScalerTrait"))
		})

		It("should find PolicyDefinition functions", func() {
			goFile := filepath.Join(tempDir, "policy.go")
			content := `package policies

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func TopologyPolicy() *defkit.PolicyDefinition {
	return defkit.NewPolicy("topology")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(HaveLen(1))
			Expect(defs[0].Name).To(Equal("topology"))
			Expect(defs[0].Type).To(Equal("policy"))
		})

		It("should find WorkflowStepDefinition functions", func() {
			goFile := filepath.Join(tempDir, "workflow.go")
			content := `package workflows

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func DeployWorkflowStep() *defkit.WorkflowStepDefinition {
	return defkit.NewWorkflowStep("deploy")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(HaveLen(1))
			Expect(defs[0].Name).To(Equal("deploy"))
			Expect(defs[0].Type).To(Equal("workflow-step"))
		})

		It("should find multiple definitions in one file", func() {
			goFile := filepath.Join(tempDir, "definitions.go")
			content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func WebserviceComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("webservice")
}

func WorkerComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("worker")
}

func ScalerTrait() *defkit.TraitDefinition {
	return defkit.NewTrait("scaler")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(HaveLen(3))
		})

		It("should ignore non-definition functions", func() {
			goFile := filepath.Join(tempDir, "mixed.go")
			content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func WebserviceComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("webservice")
}

func helperFunction() string {
	return "helper"
}

func anotherHelper(x int) int {
	return x + 1
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(HaveLen(1))
			Expect(defs[0].FunctionName).To(Equal("WebserviceComponent"))
		})

		It("should ignore methods (functions with receivers)", func() {
			goFile := filepath.Join(tempDir, "methods.go")
			content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

type Builder struct{}

func (b *Builder) Component() *defkit.ComponentDefinition {
	return defkit.NewComponent("from-method")
}

func StandaloneComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("standalone")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(HaveLen(1))
			Expect(defs[0].FunctionName).To(Equal("StandaloneComponent"))
		})

		It("should return error for invalid Go file", func() {
			goFile := filepath.Join(tempDir, "invalid.go")
			content := `this is not valid go code`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			_, err = goloader.AnalyzeGoFile(goFile)
			Expect(err).To(HaveOccurred())
		})

		It("should return empty slice for file with no definitions", func() {
			goFile := filepath.Join(tempDir, "nodefs.go")
			content := `package main

func main() {
	println("hello")
}
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs).To(BeEmpty())
		})
	})

	Describe("Definition name extraction", func() {
		It("should extract name from function name with Component suffix", func() {
			goFile := filepath.Join(tempDir, "component.go")
			content := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func WebserviceComponent() *defkit.ComponentDefinition { return nil }
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs[0].Name).To(Equal("webservice"))
		})

		It("should extract name from function name with Trait suffix", func() {
			goFile := filepath.Join(tempDir, "trait.go")
			content := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func IngressTrait() *defkit.TraitDefinition { return nil }
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs[0].Name).To(Equal("ingress"))
		})

		It("should extract name from function name with Definition suffix", func() {
			goFile := filepath.Join(tempDir, "def.go")
			content := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func DaemonDefinition() *defkit.ComponentDefinition { return nil }
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs[0].Name).To(Equal("daemon"))
		})

		It("should lowercase function name without suffix", func() {
			goFile := filepath.Join(tempDir, "simple.go")
			content := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func Webservice() *defkit.ComponentDefinition { return nil }
`
			err := os.WriteFile(goFile, []byte(content), 0600)
			Expect(err).NotTo(HaveOccurred())

			defs, err := goloader.AnalyzeGoFile(goFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(defs[0].Name).To(Equal("webservice"))
		})
	})
})
