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
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewTemplateCommand creates `template` command and its nested children command
func NewTemplateCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "template",
		DisableFlagsInUseLine: true,
		Short:                 "Manage templates",
		Long:                  "Manage templates",
		Hidden:                true,
		Annotations:           map[string]string{},
	}
	cmd.SetOut(ioStream.Out)
	cmd.AddCommand(NewTemplateContextCommand(ioStream))
	return cmd
}

// NewTemplateContextCommand creates `context` command
func NewTemplateContextCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "context",
		DisableFlagsInUseLine: true,
		Short:                 "Show context parameters",
		Long:                  "Show context parameter",
		Example:               `vela template context`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ioStream.Info(cue.BaseTemplate)
			return nil
		},
	}
	cmd.SetOut(ioStream.Out)
	return cmd
}
