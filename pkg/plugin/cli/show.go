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
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/cli"
)

// NewCapabilityShowCommand shows the reference doc for a workload type or trait
func NewCapabilityShowCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:     "show",
		Short:   "Show the reference doc for a workload type or trait",
		Long:    "Show the reference doc for a workload type or trait",
		Example: `kubectl vela show webservice`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("please specify a workload type or trait")
			}
			ctx := context.Background()
			capabilityName := args[0]
			return cli.ShowReferenceConsole(ctx, c, ioStreams, capabilityName, namespace)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypePlugin,
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "specify namespace of the definition to show")
	cmd.SetOut(ioStreams.Out)
	return cmd
}
