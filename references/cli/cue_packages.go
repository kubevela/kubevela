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

package cli

import (
	"fmt"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewCUEPackageCommand creates `cue-package` command
func NewCUEPackageCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "cue-packages",
		DisableFlagsInUseLine: true,
		Hidden:                true,
		Short:                 "List cue package",
		Long:                  "List CUE packages available.",
		Example:               `vela cue-packages`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return printCUEPackageList(c, ioStreams)
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func printCUEPackageList(c common.Args, ioStreams cmdutil.IOStreams) error {
	pd, err := c.GetPackageDiscover()
	if err != nil {
		return fmt.Errorf("get cue package discover: %w", err)
	}

	table := uitable.New()
	table.AddRow("DEFINITION-NAME", "IMPORT-PATH", " USAGE")

	packages := pd.ListPackageKinds()
	for path, kinds := range packages {
		if strings.HasPrefix(path, "kube/") {
			continue
		}
		for _, kind := range kinds {
			// TODO(yangsoon) support other kind object in future
			table.AddRow(kind.DefinitionName, path, "Kube Object for "+kind.APIVersion+"."+kind.Kind)
		}
	}
	ioStreams.Info(table.String())
	return nil
}
