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
	"os"
	"testing"

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

func TestNewTraitsCommandPersistentPreRunE(t *testing.T) {
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := common2.Args{}
	cmd := NewTraitCommand(fakeC, io)
	assert.Nil(t, cmd.PersistentPreRunE(new(cobra.Command), []string{}))
}

func TestTraitsAppliedToAllWorkloads(t *testing.T) {
	trait := types.Capability{
		Name:      "route",
		CrdName:   "routes.oam.dev",
		AppliesTo: []string{"*"},
	}
	workloads := []types.Capability{
		{
			Name:    "deployment",
			CrdName: "deployments.apps",
		},
		{
			Name:    "clonset",
			CrdName: "clonsets.alibaba",
		},
	}
	assert.Equal(t, []string{"*"}, common.ConvertApplyTo(trait.AppliesTo, workloads))
}
