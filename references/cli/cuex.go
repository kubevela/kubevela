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
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
)

// CueXCommandGroup commands for cuex management
func CueXCommandGroup(f velacmd.Factory, order string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cuex",
		Short: i18n.T("Manage CueX engine for compile."),
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeAuxiliary,
			types.TagCommandOrder: order,
		},
	}
	cmd.AddCommand(NewCueXEvalCommand(f))
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

var (
	cuexEvalLong = templates.LongDesc(i18n.T(`
		Eval cue file with CueX engine.

		Evaluate your cue file with the CueX engine. When your cue file does not
		use KubeVela's extension, it will work similarly to the native CUE CLI. 
		When using KubeVela's extensions, this command will execute the extension
		functions and resolve values, in addition to the native CUE compile process.
	`))

	cuexEvalExample = templates.Examples(i18n.T(`
		# Evaluate a cue file
		vela cuex eval -f my.cue

		# Evaluate a cue file into json format
		vela cuex eval -f my.cue -o json

		# Evaluate a cue file and output the target path 
		vela cuex eval -f my.cue -p key.path
	`))
)

// NewCueXEvalCommand `vela cuex eval` command
func NewCueXEvalCommand(f velacmd.Factory) *cobra.Command {
	opt := &CueXEvalOption{
		Format: string(util.PrintFormatCue),
	}
	cmd := &cobra.Command{
		Use:     "eval",
		Short:   i18n.T("Eval cue file with CueX engine."),
		Long:    cuexEvalLong,
		Example: cuexEvalExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opt.Run(cmd)
		},
	}
	cmd.Flags().StringVarP(&opt.Format, "format", "o", opt.Format, "format of the output")
	cmd.Flags().StringVarP(&opt.File, "file", "f", opt.File, "file for eval")
	cmd.Flags().StringVarP(&opt.Path, "path", "p", opt.Path, "path for eval")
	return velacmd.NewCommandBuilder(f, cmd).
		WithResponsiveWriter().
		Build()
}
