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
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/fatih/color"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestParseMap(t *testing.T) {
	testcase := []struct {
		args     []string
		res      map[string]interface{}
		nilError bool
	}{
		{
			args: []string{"key1=value1"},
			res: map[string]interface{}{
				"key1": "value1",
			},
			nilError: true,
		},
		{
			args: []string{"dbUrl=mongodb=mgset-58800212"},
			res: map[string]interface{}{
				"dbUrl": "mongodb=mgset-58800212",
			},
			nilError: true,
		},
		{
			args: []string{"imagePullSecrets={a,b,c}"},
			res: map[string]interface{}{
				"imagePullSecrets": []interface{}{
					"a", "b", "c",
				},
			},
			nilError: true,
		},
		{
			args: []string{"image.repo=www.test.com", "image.tag=1.1"},
			res: map[string]interface{}{
				"image": map[string]interface{}{
					"repo": "www.test.com",
					"tag":  "1.1",
				},
			},
			nilError: true,
		},
		{
			args: []string{"local=true"},
			res: map[string]interface{}{
				"local": true,
			},
			nilError: true,
		},
		{
			args: []string{"replicas=3"},
			res: map[string]interface{}{
				"replicas": int64(3),
			},
			nilError: true,
		},
	}
	for _, s := range testcase {
		r, err := parseAddonArgsToMap(s.args)
		if s.nilError {
			assert.NoError(t, err)
			assert.Equal(t, s.res, r)
		} else {
			assert.Error(t, err, fmt.Sprintf("%v should be error case", s.args))
		}
	}
}

func TestAddonEnableCmdWithErrLocalPath(t *testing.T) {
	testcase := []struct {
		args   []string
		errMsg string
	}{
		{
			args:   []string{"./a_local_path"},
			errMsg: "addon directory ./a_local_path not found in local",
		},
		{
			args:   []string{"a_local_path/"},
			errMsg: "addon directory a_local_path/ not found in local",
		},
	}

	ioStream := util.IOStreams{}
	commandArgs := common.Args{}
	cmd := NewAddonEnableCommand(commandArgs, ioStream)
	initCommand(cmd)

	for _, s := range testcase {
		cmd.SetArgs(s.args)
		err := cmd.Execute()
		assert.Error(t, err, s.errMsg)
	}
}

var _ = Describe("Test AddonRegistry Cmd", func() {
	It("Test AddonRegistryAddCmd", func() {
		testAddonRegistryAddCmd()
	})
})

func testAddonRegistryAddCmd() {
	testcase := []struct {
		args   []string
		errMsg string
		result *pkgaddon.Registry
	}{
		{
			args:   []string{"noAuthRegistry", "--type=helm", "--endpoint=http://127.0.0.1/chartrepo/oam"},
			errMsg: "fail to add no auth addon registry",
		},
		{
			args:   []string{"basicAuthRegistry", "--type=helm", "--endpoint=http://127.0.0.1/chartrepo/oam", "--username=hello", "--password=word"},
			errMsg: "fail to add basis auth addon registry",
		},
		{
			args: []string{"skipTlsRegistry", "--type=helm", "--endpoint=https://127.0.0.1/chartrepo/oam", "--insecureSkipTLS=true"},
		},
	}

	ioStream := util.IOStreams{}
	commandArgs := common.Args{}
	commandArgs.SetClient(k8sClient)

	cmd := NewAddAddonRegistryCommand(commandArgs, ioStream)

	for _, s := range testcase {
		cmd.SetArgs(s.args)
		err := cmd.Execute()
		Expect(err).To(HaveOccurred())
	}
}

func TestAddonUpgradeCmdWithErrLocalPath(t *testing.T) {
	testcase := []struct {
		args   []string
		errMsg string
	}{
		{
			args:   []string{"./a_local_path"},
			errMsg: "addon directory ./a_local_path not found in local",
		},
		{
			args:   []string{"a_local_path/"},
			errMsg: "addon directory a_local_path/ not found in local",
		},
	}

	ioStream := util.IOStreams{}
	commandArgs := common.Args{}
	cmd := NewAddonUpgradeCommand(commandArgs, ioStream)
	initCommand(cmd)

	for _, s := range testcase {
		cmd.SetArgs(s.args)
		err := cmd.Execute()
		assert.Error(t, err, s.errMsg)
	}
}

func TestTransCluster(t *testing.T) {
	testcase := []struct {
		str string
		res []interface{}
	}{
		{
			str: "{cluster1, cluster2}",
			res: []interface{}{"cluster1", "cluster2"},
		},
		{
			str: "{cluster1,cluster2}",
			res: []interface{}{"cluster1", "cluster2"},
		},
		{
			str: "{cluster1,  cluster2   }",
			res: []interface{}{"cluster1", "cluster2"},
		},
	}
	for _, s := range testcase {
		assert.Equal(t, transClusters(s.str), s.res)
	}
}

func TestGenerateAvailableVersions(t *testing.T) {
	type testcase struct {
		inVersion string
		versions  []string
	}
	testcases := []struct {
		c   testcase
		res string
	}{
		{
			c: testcase{
				inVersion: "1.2.1",
				versions:  []string{"1.2.1"},
			},
			res: fmt.Sprintf("[%s]", color.New(color.Bold, color.FgGreen).Sprintf("1.2.1")),
		},
		{
			c: testcase{
				inVersion: "1.2.1",
				versions:  []string{"1.2.3", "1.2.2", "1.2.1"},
			},
			res: fmt.Sprintf("[%s, 1.2.3, 1.2.2]", color.New(color.Bold, color.FgGreen).Sprintf("1.2.1")),
		},
		{
			c: testcase{
				inVersion: "1.2.1",
				versions:  []string{"1.2.3", "1.2.2", "1.2.1", "1.2.0"},
			},
			res: fmt.Sprintf("[%s, 1.2.3, 1.2.2, ...]", color.New(color.Bold, color.FgGreen).Sprintf("1.2.1")),
		},
	}
	for _, s := range testcases {
		re := genAvailableVersionInfo(s.c.versions, s.c.inVersion, 3)
		assert.Equal(t, re, s.res)
	}
}

func TestLimitStringLength(t *testing.T) {
	type testcase struct {
		testString  string
		lengthLimit int
	}

	testcases := []struct {
		c   testcase
		res string
	}{
		// len = limit
		{
			c: testcase{
				testString:  "4444",
				lengthLimit: 4,
			},
			res: "4444",
		},
		// len > limit
		{
			c: testcase{
				testString:  "3333",
				lengthLimit: 3,
			},
			res: "333...",
		},
		// len < limit
		{
			c: testcase{
				testString:  "22",
				lengthLimit: 3,
			},
			res: "22",
		},
		// limit = 0
		{
			c: testcase{
				testString:  "000",
				lengthLimit: 0,
			},
			res: "000",
		},
		// limit < 0
		{
			c: testcase{
				testString:  "000",
				lengthLimit: -1,
			},
			res: "000",
		},
	}

	for _, s := range testcases {
		re := limitStringLength(s.c.testString, s.c.lengthLimit)
		assert.Equal(t, re, s.res)
	}
}

func TestAddonPackageCmdWithInvalidArgs(t *testing.T) {
	testcase := []struct {
		args []string
		msg  string
	}{
		{
			args: []string{},
			msg:  "must specify addon directory path",
		},
		{
			args: []string{"./a_local_path"},
			msg:  "fail to package",
		},
		{
			args: []string{"a_local_path/"},
			msg:  "fail to package",
		},
	}

	commandArgs := common.Args{}
	cmd := NewAddonPackageCommand(commandArgs)

	for _, s := range testcase {
		cmd.SetArgs(s.args)
		err := cmd.Execute()
		assert.ErrorContains(t, err, s.msg)
	}
}

func TestPackageValidAddon(t *testing.T) {
	commandArgs := common.Args{}
	cmd := NewAddonPackageCommand(commandArgs)
	cmd.SetArgs([]string{"./test-data/addon/sample"})
	err := cmd.Execute()
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll("sample-1.0.1.tgz")
	}()
}

func TestGenerateParameterString(t *testing.T) {
	testcase := []struct {
		status       pkgaddon.Status
		addonPackage *pkgaddon.WholeAddonPackage
		outputs      []string
	}{
		{
			status: pkgaddon.Status{},
			addonPackage: &pkgaddon.WholeAddonPackage{
				APISchema: nil,
			},
			outputs: []string{""},
		},
		{
			status: pkgaddon.Status{
				Parameters: map[string]interface{}{
					"database": "kubevela",
					"dbType":   "kubeapi",
				},
			},
			addonPackage: &pkgaddon.WholeAddonPackage{
				APISchema: &openapi3.Schema{
					Required: []string{"dbType", "serviceAccountName", "serviceType", "dex"},
					Properties: openapi3.Schemas{
						"database": &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Description: "Specify the database name, for the kubeapi db type, it represents namespace.",
								Default:     nil,
							},
						},
						"dbURL": &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Description: "Specify the MongoDB URL. it only enabled where DB type is MongoDB.",
								Default:     "abc.com",
							},
						},
						"dbType": &openapi3.SchemaRef{
							Value: &openapi3.Schema{
								Description: "Specify the database type, current support KubeAPI(default) and MongoDB.",
								Enum:        []interface{}{"kubeapi", "mongodb"},
							},
						},
					},
				},
			},
			outputs: []string{
				// dbType
				color.New(color.FgCyan).Sprintf("-> ") +
					color.New(color.Bold).Sprint("dbType") + ": " +
					"Specify the database type, current support KubeAPI(default) and MongoDB.\n" +
					"\tcurrent value: " + color.New(color.FgGreen).Sprint("\"kubeapi\"\n") +
					"\trequired: " + color.GreenString("✔\n") +
					"\toptions: \"kubeapi\", \"mongodb\"\n",
				// dbURL
				color.New(color.FgCyan).Sprintf("-> ") +
					color.New(color.Bold).Sprint("dbURL") + ": " +
					"Specify the MongoDB URL. it only enabled where DB type is MongoDB.\n" +
					"\tdefault: " + "\"abc.com\"\n",
				// database
				color.New(color.FgCyan).Sprintf("-> ") +
					color.New(color.Bold).Sprint("database") + ": " +
					"Specify the database name, for the kubeapi db type, it represents namespace.\n" +
					"\tcurrent value: " + color.New(color.FgGreen).Sprint("\"kubevela\""),
			},
		},
	}

	for _, s := range testcase {
		res := generateParameterString(s.status, s.addonPackage)
		for _, o := range s.outputs {
			assert.Contains(t, res, o)
		}

	}
}

func TestNewAddonCreateCommand(t *testing.T) {
	cmd := NewAddonInitCommand()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.ErrorContains(t, err, "required")

	cmd.SetArgs([]string{"--chart", "a", "--helm-repo", "https://some.com", "--chart-version", "c"})
	err = cmd.Execute()
	assert.ErrorContains(t, err, "required")

	cmd.SetArgs([]string{"test-addon", "--chart", "a", "--helm-repo", "https://some.com", "--chart-version", "c"})
	err = cmd.Execute()
	assert.NoError(t, err)
	_ = os.RemoveAll("test-addon")

	cmd.SetArgs([]string{"test-addon"})
	err = cmd.Execute()
	assert.NoError(t, err)
	_ = os.RemoveAll("test-addon")

}

func TestCheckSpecifyRegistry(t *testing.T) {
	testCases := []struct {
		name      string
		registry  string
		addonName string
		hasError  bool
	}{
		{
			name:      "fluxcd",
			registry:  "",
			addonName: "fluxcd",
			hasError:  false,
		},
		{
			name:      "kubevela/fluxcd",
			registry:  "kubevela",
			addonName: "fluxcd",
			hasError:  false,
		},
		{
			name:      "test/kubevela/fluxcd",
			registry:  "",
			addonName: "",
			hasError:  true,
		},
	}
	for _, testCase := range testCases {
		r, n, err := splitSpecifyRegistry(testCase.name)
		assert.Equal(t, err != nil, testCase.hasError)
		assert.Equal(t, r, testCase.registry)
		assert.Equal(t, n, testCase.addonName)
	}
}
