package common

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

var (
	Port           = ":58081"
	ResponseString = "Hello HTTP Get."
)

func TestInitBaseRestConfig(t *testing.T) {
	args, err := InitBaseRestConfig()
	assert.NotNil(t, t, args)
	assert.NoError(t, err)
}

func TestHTTPGet(t *testing.T) {
	type want struct {
		data   string
		errStr string
	}
	var (
		ctx         = context.Background()
		abnormalUrl = "http://abc.def"
	)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, ResponseString)
	}))
	defer testServer.Close()

	cases := map[string]struct {
		reason string
		url    string
		want   want
	}{
		"abnormal case": {
			reason: "url is invalid",
			url:    abnormalUrl,
			want: want{
				data:   "",
				errStr: fmt.Sprintf("Get \"%s\": EOF", abnormalUrl),
			},
		},
		"normal case": {
			reason: "url is valid\n",
			url:    testServer.URL,
			want: want{
				data:   fmt.Sprintf("%s\n", ResponseString),
				errStr: "",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := HTTPGet(ctx, tc.url)
			if tc.want.errStr != "" {
				if diff := cmp.Diff(tc.want.errStr, err.Error(), test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nHTTPGet(...): -want error, +got error:\n%s", tc.reason, diff)
				}
			}

			if diff := cmp.Diff(tc.want.data, string(got)); diff != "" {
				t.Errorf("\n%s\nHTTPGet(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}

}

func TestGetCUEParameterValue(t *testing.T) {
	type want struct {
		err error
	}
	var validCueStr = `
parameter: {
	min: int
}
`

	var CueStrNotContainParameter = `
output: {
	min: int
}
`
	cases := map[string]struct {
		reason string
		cueStr string
		want   want
	}{
		"GetCUEParameterValue": {
			reason: "cue string is valid",
			cueStr: validCueStr,
			want: want{
				err: nil,
			},
		},
		"CUEStringNotContainParameter": {
			reason: "cue string doesn't contain Parameter",
			cueStr: CueStrNotContainParameter,
			want: want{
				err: fmt.Errorf("parameter not exist"),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := GetCUEParameterValue(tc.cueStr)
			if tc.want.err != nil {
				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want error, +got error:\n%s", tc.reason, diff)
				}
			}

		})
	}
}

func TestGetCUEParameterValue4RareCases(t *testing.T) {
	type want struct {
		errMsg string
	}

	var invalidCueStr = `
name
`
	cases := map[string]struct {
		reason string
		cueStr string
		want   want
	}{
		"CUEStringInvalid": {
			reason: "cue string is invalid",
			cueStr: invalidCueStr,
			want: want{
				errMsg: "reference \"name\" not found",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := GetCUEParameterValue(tc.cueStr)
			if diff := cmp.Diff(tc.want.errMsg, err.Error(), test.EquateConditions()); diff != "" {
				t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

		})
	}
}

func TestGenOpenAPI(t *testing.T) {
	type want struct {
		data []byte
		err  error
	}
	var dir = "testdata"
	var validCueFile = "workload1.cue"
	var validTargetSchema = "workload1.json"
	targetFile := filepath.Join(dir, validTargetSchema)
	expect, _ := ioutil.ReadFile(targetFile)

	normalWant := want{
		data: expect,
		err:  nil,
	}

	f := filepath.FromSlash(validCueFile)

	inst := cue.Build(load.Instances([]string{f}, &load.Config{
		Dir: dir,
	}))[0]

	cases := map[string]struct {
		reason       string
		fileDir      string
		fileName     string
		targetSchema string
		want         want
	}{
		"GenOpenAPI": {
			reason:       "generate OpenAPI schema",
			fileDir:      dir,
			fileName:     validCueFile,
			targetSchema: validTargetSchema,
			want:         normalWant,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := GenOpenAPI(inst)
			if tc.want.err != nil {
				if diff := cmp.Diff(tc.want.err, errors.New(err.Error()), test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want error, +got error:\n%s", tc.reason, diff)
				}
			}

			if diff := cmp.Diff(tc.want.data, got); diff != "" {
				t.Errorf("\n%s\nGenOpenAPIFromFile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestRealtimePrintCommandOutput(t *testing.T) {
	cmd := exec.Command("bash", "-c", "date")
	err := RealtimePrintCommandOutput(cmd, "")
	assert.NoError(t, err)
	cmd.Process.Kill()

	var logFile = "terraform.log"
	var hello = "Hello"
	cmd = exec.Command("bash", "-c", fmt.Sprintf("echo \"%s\"", hello))
	err = RealtimePrintCommandOutput(cmd, logFile)
	assert.NoError(t, err)

	data, _ := ioutil.ReadFile(logFile)
	assert.Contains(t, string(data), hello)
	os.Remove(logFile)
}
