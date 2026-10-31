package definitions_test

import (
	"bytes"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	velaTemplatesDir     = "../../vela-templates/definitions/"
	testDataDir          = "./test-data/"
	cueExtension         = ".cue"
	testDataSeparateFlag = "// -separate"
)

var (
	inputKeys    = []string{"parameter", "context"}
	expectedKeys = []string{"outputs", "output"}
)

func TestDefinitions(t *testing.T) {
	// 1. traverse all definition .cue file
	var targets []string
	filepath.Walk(velaTemplatesDir, func(path string, info fs.FileInfo, err error) error {
		// TODO omit workflowstep dir for feature support
		if info.IsDir() && info.Name() == "workflowstep" {
			return nil
		}
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
	// split inputs and outputs
	inputBytes := bytes.Split(testdataBytes, []byte(testDataSeparateFlag))[0]
	inputV := cuectx.CompileBytes(inputBytes)
	outputV := cuectx.CompileBytes(testdataBytes)

	// 2. parse target definition .cue file
	targetBytes, err := os.ReadFile(targetPath)
	if err != nil {
		assert.Errorf(t, err, "failed to read defnition .cue file %s", targetPath)
	}
	// TODO append all contents in the test file into target file
	targetV := cuectx.CompileBytes(append(targetBytes, inputBytes...))

	// 3. compare expected outputs to actual outputs
	for _, expectedKey := range expectedKeys {
		expectedV := outputV.LookupPath(cue.ParsePath(expectedKey))
		if !expectedV.Exists() {
			continue
		}
		expectedJsonV, err := expectedV.MarshalJSON()
		assert.Nilf(t, err, "failed to parse test file %s", testPath)

		actualV := targetV.LookupPath(cue.ParsePath("template")).Unify(inputV).
			LookupPath(cue.ParsePath(expectedKey))
		actualJsonV, err := actualV.MarshalJSON()

		if assert.JSONEqf(t, string(expectedJsonV), string(actualJsonV),
			"failed to generate identical outputs for test case %s", testPath) {
			fmt.Printf("[test definition] %s pass\n", testPath)
		}
	}
}

func TestUnify(t *testing.T) {
	var (
		a = `
template: {
	outputs: {
		port: parameter.port
		annotations: "test": "value"
	}
	parameter:port: int
}
`
		b = `
parameter:port: 8080 

`
		expected = `
{"port":8080,"annotations":{"test":"value"}}
`
	)

	cuectx := cuecontext.New()
	v := cuectx.CompileString(a)
	inputV := cuectx.CompileString(b)

	assert.True(t, v.Exists())

	vJson, err := v.LookupPath(cue.ParsePath("template")).
		Unify(inputV).LookupPath(cue.ParsePath("outputs")).MarshalJSON()
	assert.Nil(t, err)

	assert.JSONEq(t, string(vJson), expected)
}
