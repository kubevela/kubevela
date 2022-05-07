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
	"strings"
	"testing"

	"github.com/fatih/color"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"

	"gotest.tools/assert"
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
			assert.NilError(t, err)
			assert.DeepEqual(t, s.res, r)
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

	for _, s := range testcase {
		cmd.SetArgs(s.args)
		err := cmd.Execute()
		assert.Error(t, err, s.errMsg)
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

	for _, s := range testcase {
		cmd.SetArgs(s.args)
		err := cmd.Execute()
		assert.Error(t, err, s.errMsg)
	}
}

func TestTransCluster(t *testing.T) {
	testcase := []struct {
		str string
		res []string
	}{
		{
			str: "{cluster1, cluster2}",
			res: []string{"cluster1", "cluster2"},
		},
		{
			str: "{cluster1,cluster2}",
			res: []string{"cluster1", "cluster2"},
		},
		{
			str: "{cluster1,  cluster2   }",
			res: []string{"cluster1", "cluster2"},
		},
	}
	for _, s := range testcase {
		assert.DeepEqual(t, transClusters(s.str), s.res)
	}
}

func TestGenerateStatusIn(t *testing.T) {
	testcases := []struct {
		c   pkgaddon.Status
		res []string
	}{
		{
			c:   pkgaddon.Status{InstalledVersion: "1.2.1", Clusters: map[string]map[string]interface{}{"cluster1": nil, "cluster2": nil}, AddonPhase: statusEnabled},
			res: []string{"installedVersion: 1.2.1", "installedClusters: [cluster1 cluster2]", fmt.Sprintf("status is %s", color.New(color.FgGreen).Sprintf(statusEnabled))},
		},
		{
			c:   pkgaddon.Status{InstalledVersion: "1.2.3", AddonPhase: statusSuspend},
			res: []string{"installedVersion: 1.2.3", fmt.Sprintf("status is %s", color.New(color.FgRed).Sprintf(statusSuspend))},
		},
	}
	for _, testcase := range testcases {
		res := generateAddonInfo("test", testcase.c)
		for _, re := range testcase.res {
			assert.Equal(t, strings.Contains(res, re), true)
		}
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
		re := genAvailableVersionInfo(s.c.versions, s.c.inVersion)
		assert.Equal(t, re, s.res)
	}
}
