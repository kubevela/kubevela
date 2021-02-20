package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/utils/system"
)

const TestDir = "testdata/definition"

func TestGenerateOpenAPISchemaFromCapabilityParameter(t *testing.T) {

	var invalidWorkloadName = "IAmAnInvalidWorkloadDefinition"
	capabilityDir, _ := system.GetCapabilityDir()
	if _, err := os.Stat(capabilityDir); err != nil && os.IsNotExist(err) {
		os.MkdirAll(capabilityDir, 0755)
	}
	invalidWorkloadPath := filepath.Join(capabilityDir, fmt.Sprintf("%s.cue", invalidWorkloadName))

	type want struct {
		data []byte
		err  error
	}

	cases := map[string]struct {
		reason string
		name   string
		want   want
	}{
		"GenerateOpenAPISchemaFromInvalidCapability": {
			reason: "generate OpenAPI schema for an invalid Workload/Trait",
			name:   invalidWorkloadName,
			want:   want{data: nil, err: &os.PathError{Op: "open", Path: invalidWorkloadPath, Err: fmt.Errorf("no such file or directory")}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := generateOpenAPISchemaFromCapabilityParameter(tc.name)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetDefinition(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.data, got); diff != "" {
				t.Errorf("\n%s\ngetDefinition(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestPrepareParameterCue(t *testing.T) {
	targetSchemaDir := filepath.Join(TestDir, "openapi")
	var noParameterCueName = "workloadNoParameter.cue"
	var err = fmt.Errorf("cue file %s doesn't contain section `parmeter`", filepath.Join(TestDir, noParameterCueName))

	var invalidDir = "IAmAnInvalidDirectory/openapi"
	var invalidDirErr = &os.PathError{Op: "mkdir", Path: invalidDir, Err: fmt.Errorf("no such file or directory")}

	var invalidSourceFile = "IAmAnInvalidFile"
	invalidSourceFilePath := filepath.Join(TestDir, invalidSourceFile)
	var invalidSourceFileErr = &os.PathError{Op: "open", Path: invalidSourceFilePath, Err: fmt.Errorf("no such file or directory")} //
	// fmt.Errorf("open %s: no such file or directory", )
	cases := map[string]struct {
		reason          string
		fileDir         string
		fileName        string
		targetSchemaDir string
		want            error
	}{
		"PrepareANormalParameterCueFile": {
			reason:          "Prepare a normal parameter cue file",
			fileDir:         TestDir,
			fileName:        "workload1.cue",
			targetSchemaDir: targetSchemaDir,
			want:            nil,
		},
		"CueFileNotContainParameter": {
			reason:          "Prepare a cue file which doesn't contain `parameter` section",
			fileDir:         TestDir,
			fileName:        noParameterCueName,
			targetSchemaDir: targetSchemaDir,
			want:            err,
		},
		"InvalidTargetSchemaDir": {
			reason:          "target schema directory is invalid",
			fileDir:         "",
			fileName:        "",
			targetSchemaDir: invalidDir,
			want:            invalidDirErr,
		},
		"InvalidSourceCueFile": {
			reason:          "source cue file is invalid",
			fileDir:         TestDir,
			fileName:        invalidSourceFile,
			targetSchemaDir: TestDir,
			want:            invalidSourceFileErr,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := prepareParameterCue(tc.fileDir, tc.fileName, tc.targetSchemaDir)
			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nprepareParameterCue(...): -want error, +got error:\n%s", tc.reason, diff)
			}

		})
	}
	os.RemoveAll(targetSchemaDir)
}

func TestAppendCueReference(t *testing.T) {
	var cueStr = `
#parameter: {
	min: int
}
`
	temporaryDir := filepath.Join(TestDir, "temp")
	os.Mkdir(temporaryDir, 0750)
	var cueFile = filepath.Join(temporaryDir, "workloadPureParameter.cue")
	ioutil.WriteFile(cueFile, []byte(cueStr), 0750)
	cases := map[string]struct {
		reason  string
		cueFile string
		want    error
	}{
		"AppendCueReference": {
			reason:  "Append Cue Reference",
			cueFile: cueFile,
			want:    nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := appendCueReference(tc.cueFile)
			if (err == nil && tc.want != nil) || (err != nil && tc.want == nil) {
				t.Errorf("%s\nappendCueReference(...): -want %s, +got %s", tc.reason, tc.want, err)
			}
			data, err := ioutil.ReadFile(tc.cueFile)
			if err != nil {
				t.Errorf("%s\nappendCueReference(...): target file %s could not be read: %s", tc.reason, tc.cueFile, err)
			}
			if strings.HasSuffix(string(data), "context: {") {
				t.Errorf("%s\nappendCueReference(...): target file %s doesn't contain `parameter` section", tc.reason, tc.cueFile)
			}
		})
	}
	os.RemoveAll(temporaryDir)
}

func TestFixOpenAPISchema(t *testing.T) {
	cases := map[string]struct {
		inputFile string
		fixedFile string
	}{
		"StandardWorkload": {
			inputFile: "webservice.json",
			fixedFile: "webserviceFixed.json",
		},
		"ShortTagJson": {
			inputFile: "shortTagSchema.json",
			fixedFile: "shortTagSchemaFixed.json",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			swagger, _ := openapi3.NewSwaggerLoader().LoadSwaggerFromFile(filepath.Join(TestDir, tc.inputFile))
			schema := swagger.Components.Schemas["parameter"].Value
			fixOpenAPISchema("", schema)
			fixedSchema, _ := schema.MarshalJSON()
			expectedSchema, _ := ioutil.ReadFile(filepath.Join(TestDir, tc.fixedFile))
			assert.Equal(t, fixedSchema, expectedSchema)
		})
	}
}

func TestGetParameterItemName(t *testing.T) {
	got, err := getParameterItemName("    \"cmd\": {\n")
	name := "cmd"
	assert.NoError(t, err)
	assert.Equal(t, name, got)
}

func TestGetParameterFromOpenAPISchema(t *testing.T) {
	cases := map[string]struct {
		reason     string
		fileDir    string
		fileName   string
		targetFile string
		want       error
	}{
		"GetParameterFromOpenAPISchema": {
			reason:     "get parameter from OpenAPI schema",
			fileDir:    TestDir,
			fileName:   "normalOpenAPISchemaFixed.json",
			targetFile: "normalOpenAPISchemaParameter.json",
			want:       nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rawFile := filepath.Join(tc.fileDir, tc.fileName)
			raw, err := ioutil.ReadFile(rawFile)
			if err != nil {
				t.Errorf("%s\ngetParameterFromOpenAPISchema(...): raw file %s could not be read: %s", tc.reason, rawFile, err)
			}
			got, err := getParameterFromOpenAPISchema(raw)
			if (err == nil && tc.want != nil) || (err != nil && tc.want == nil) {
				t.Errorf("%s\ngetParameterFromOpenAPISchema(...): -want %s, +got %s", tc.reason, tc.want, err)
			}
			targetFile := filepath.Join(tc.fileDir, tc.targetFile)
			expect, err := ioutil.ReadFile(targetFile)
			if err != nil {
				t.Errorf("%s\ngetParameterFromOpenAPISchema(...): target file %s could not be read: %s", tc.reason, targetFile, err)
			}
			if strings.Compare(string(got), string(expect)) != 0 {
				t.Errorf("%s\ngetParameterFromOpenAPISchema(...): pure parameter schema %s isn't retrieved", tc.reason, targetFile)
			}
		})
	}
}
