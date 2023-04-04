package definitions_tests

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	velaTemplatesDir   = "../vela-templates/definitions/"
	testDataDir        = "./test-data/"
	expectedOutputsKey = "outputs"
	cueExtension       = ".cue"
)

func TestDefinitions(t *testing.T) {
	// 1. traverse all definition .cue file
	var targets []string
	filepath.Walk(velaTemplatesDir, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(info.Name(), cueExtension) {
			targets = append(targets, strings.TrimPrefix(path, velaTemplatesDir))
		}
		return nil
	})

	// 2. lookup test data based on the same relative filepath
	for _, targetPath := range targets {
		testSubDir := filepath.Join(testDataDir, targetPath[:strings.LastIndex(targetPath, cueExtension)])
		_, err := os.Stat(testSubDir)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(testSubDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), cueExtension) {
				doTest(t, filepath.Join(testSubDir, entry.Name()), filepath.Join(velaTemplatesDir, targetPath))
			}
		}
	}
}

func doTest(t *testing.T, testPath, targetPath string) {
	// 1. parse test data
	cuectx := cuecontext.New()
	testdataBytes, err := os.ReadFile(testPath)
	if err != nil {
		assert.Errorf(t, err, "failed to read test .cue file %s", testPath)
	}
	testV := cuectx.CompileBytes(testdataBytes)
	expectedOutputsV := testV.LookupPath(cue.ParsePath(expectedOutputsKey))
	expectedOutputsJsonV, err := expectedOutputsV.MarshalJSON()

	// 2. parse target definition .cue file
	targetBytes, err := os.ReadFile(targetPath)
	if err != nil {
		assert.Errorf(t, err, "failed to read defnition .cue file %s", targetPath)
	}
	// TODO append all contents in the test file into target file
	targetV := cuectx.CompileBytes(append(targetBytes, testdataBytes...))
	actualOutputsV := targetV.LookupPath(cue.ParsePath("template.outputs"))
	actualOutputsJsonV, err := actualOutputsV.MarshalJSON()

	// 3. compare expected outputs to actual outputs
	assert.JSONEqf(t, string(expectedOutputsJsonV), string(actualOutputsJsonV),
		"failed to generate identical outputs for test case %s", testPath)
}
