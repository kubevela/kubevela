/*
 Copyright 2021. The KubeVela Authors.

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

package velaql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVelaQL(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		ql    string
		query QueryView
		err   error
	}{{
		ql:  `view{test=,test1=hello}.output`,
		err: errors.New("key or value in parameter shouldn't be empty"),
	}, {
		ql:  `{test=1,app="name"}.Export`,
		err: errors.New("fail to parse the velaQL"),
	}, {
		ql:  `view.{test=true}.output.value.spec"`,
		err: errors.New("fail to parse the velaQL"),
	}, {
		ql: `view{test=1,app="name"}`,
		query: QueryView{
			View:   "view",
			Export: "status",
		},
		err: nil,
	}, {
		ql: `view{test=1,app="name"}.Export`,
		query: QueryView{
			View:   "view",
			Export: "Export",
		},
		err: nil,
	}, {
		ql: `view{test=true}.output.value.spec`,
		query: QueryView{
			View:   "view",
			Export: "output.value.spec",
		},
		err: nil,
	}, {
		ql: `view{test=true}.output.value[0].spec`,
		query: QueryView{
			View:   "view",
			Export: "output.value[0].spec",
		},
		err: nil,
	}}

	for i, testcase := range testcases {
		t.Run(fmt.Sprintf("testcase-%d", i), func(t *testing.T) {
			q, err := ParseVelaQL(testcase.ql)
			if testcase.err != nil {
				assert.EqualError(t, testcase.err, err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testcase.query.View, q.View)
				assert.Equal(t, testcase.query.Export, q.Export)
			}
		})
	}
}

func TestParseParameter(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		parameter    string
		parameterMap map[string]interface{}
		err          error
	}{{
		parameter: `{    }`,
		err:       errors.New("parameter shouldn't be empty"),
	}, {
		parameter: `{}`,
		err:       errors.New("parameter shouldn't be empty"),
	}, {
		parameter: `{ testString = "pod" , testFloat= , testBoolean=true}`,
		err:       errors.New("key or value in parameter shouldn't be empty"),
	}, {
		parameter: `{testString="pod",testFloat=1000.10,testBoolean=true,testInt=1}`,
		parameterMap: map[string]interface{}{
			"testString":  "pod",
			"testFloat":   1000.1,
			"testBoolean": true,
			"testInt":     int64(1),
		},
		err: nil,
	}, {
		parameter: `{testString="pod",testFloat=1000.10,testBoolean=true,testInt=1,}`,
		parameterMap: map[string]interface{}{
			"testString":  "pod",
			"testFloat":   1000.1,
			"testBoolean": true,
			"testInt":     int64(1),
		},
		err: nil,
	}, {
		parameter: `{ testString = "pod" , testFloat=1000.10 , testBoolean=true , testInt=1, }`,
		parameterMap: map[string]interface{}{
			"testString":  "pod",
			"testFloat":   1000.1,
			"testBoolean": true,
			"testInt":     int64(1),
		},
		err: nil,
	}}

	for i, testcase := range testcases {
		t.Run(fmt.Sprintf("testcase-%d", i), func(t *testing.T) {
			result, err := ParseParameter(testcase.parameter)
			if testcase.err != nil {
				assert.EqualError(t, testcase.err, err.Error())
			} else {
				assert.NoError(t, err)
				for k, v := range result {
					assert.Equal(t, testcase.parameterMap[k], v)
				}
			}
		})
	}
}

func TestParseVelaQLFromPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	testdataDir := "testdata"

	testcases := []struct {
		name           string
		path           string
		expectedExport string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "Simple valid CUE file with export field",
			path:           filepath.Join(testdataDir, "simple-valid.cue"),
			expectedExport: "output.message",
			expectError:    false,
		},
		{
			name:           "Simple valid CUE file without export field",
			path:           filepath.Join(testdataDir, "simple-no-export.cue"),
			expectedExport: DefaultExportValue,
			expectError:    false,
		},
		{
			name:          "Nonexistent file path",
			path:          filepath.Join(testdataDir, "nonexistent.cue"),
			expectError:   true,
			errorContains: "read view file from",
		},
		{
			name:          "Empty file path",
			path:          "",
			expectError:   true,
			errorContains: "read view file from",
		},
		{
			name:          "Invalid CUE content",
			path:          filepath.Join(testdataDir, "invalid-cue-content.cue"),
			expectError:   true,
			errorContains: "error when parsing view",
		},
		{
			name:           "File with invalid export type - should fallback to default",
			path:           filepath.Join(testdataDir, "invalid-export.cue"),
			expectedExport: DefaultExportValue,
			expectError:    false,
		},
		{
			name:           "Empty CUE file",
			path:           filepath.Join(testdataDir, "empty.cue"),
			expectedExport: DefaultExportValue,
			expectError:    false,
		},
		{
			name:           "File with leading/trailing whitespace",
			path:           filepath.Join(testdataDir, "whitespace.cue"),
			expectedExport: "output.message",
			expectError:    false,
		},
		{
			name:          "Relative path",
			path:          "testdata/nonexistent.cue",
			expectError:   true,
			errorContains: "read view file from",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := ParseVelaQLFromPath(ctx, tc.path)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)

				if tc.path != "" {
					expectedContent, readErr := os.ReadFile(tc.path)
					require.NoError(t, readErr)
					assert.Equal(t, string(expectedContent), result.View)
				}

				assert.Equal(t, tc.expectedExport, result.Export)
				assert.Nil(t, result.Parameter)
			}
		})
	}
}
