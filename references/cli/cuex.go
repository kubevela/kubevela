/*
Copyright 2023 The KubeVela Authors.

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

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/cue/util"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
)

// CueXCommandGroup commands for cuex management
func CueXCommandGroup() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cuex",
		Short: i18n.T("Manage CueX engine for compile."),
	}
	cmd.AddCommand(NewCueXEvalCommand())
	return cmd
}

// CueXEvalOption option for compile cuex
type CueXEvalOption struct {
	Format string
	Path   string
	File   string
}

// Run compile cuex
func (in *CueXEvalOption) Run(cmd *cobra.Command) error {
	if in.File == "" {
		return fmt.Errorf("file must be provided for compile")
	}
	bs, err := os.ReadFile(in.File)
	if err != nil {
		return err
	}
	val, err := cuex.CompileString(cmd.Context(), string(bs))
	if err != nil {
		return err
	}
	bs, err = util.Print(val, util.WithFormat(in.Format), util.WithPath(in.Path))
	if err != nil {
		return err
	}
	cmd.Println(string(bs))
	return nil
}

// NewCueXEvalCommand `vela cuex eval` command
func NewCueXEvalCommand() *cobra.Command {
	opt := &CueXEvalOption{
		Format: string(util.PrintFormatCue),
	}
	cmd := &cobra.Command{
		Use:   "eval",
		Short: i18n.T("Eval cue file with CueX engine."),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opt.Run(cmd)
		},
	}
	cmd.Flags().StringVarP(&opt.Format, "format", "o", opt.Format, "format of the output")
	cmd.Flags().StringVarP(&opt.File, "file", "f", opt.File, "file for eval")
	cmd.Flags().StringVarP(&opt.Path, "path", "p", opt.Path, "path for eval")
	return cmd
}
