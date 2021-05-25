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
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/cli"
)

// NewTraitCommand creates `trait` command
func NewTraitCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "trait",
		DisableFlagsInUseLine: true,
		Short:                 "Show traits",
		Long:                  "Show installed and remote trait",
		Example:               "kubectl vela trait",
		RunE: func(cmd *cobra.Command, args []string) error {
			isDiscover, _ := cmd.Flags().GetBool("discover")
			url, _ := cmd.PersistentFlags().GetString("url")
			err := cli.PrintTraitListFromRegistry(isDiscover, url, ioStreams)
			return err
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypePlugin,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.AddCommand(
		NewTraitGetCommand(c, ioStreams),
	)
	cmd.Flags().Bool("discover", false, "discover traits in registries")
	cmd.PersistentFlags().String("url", cli.DefaultRegistry, "specify the registry URL")
	return cmd
}

// NewTraitGetCommand creates `trait get` command
func NewTraitGetCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <trait>",
		Short:   "get trait from registry",
		Long:    "get trait from registry",
		Example: "kubectl vela trait get <trait>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ioStreams.Error("you must specify the trait name")
				return nil
			}
			name := args[0]
			url, _ := cmd.Flags().GetString("url")

			return cli.InstallTraitByName(c, ioStreams, name, url)
		},
	}
	return cmd
}
