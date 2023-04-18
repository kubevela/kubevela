package cli

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	cuetoken "cuelang.org/go/cue/token"
	"cuelang.org/go/pkg/encoding/yaml"
	"encoding/json"
	"fmt"
	"github.com/oam-dev/kubevela/apis/types"
	kubecuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/spf13/cobra"
	"os"
)

var (
	defaultCuexCompiler = kubecuex.KubeVelaDefaultCompiler.Get()
)

func NewCuexCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cuex",
		Short:   "Execute cuex compiler",
		Long:    "Leverage cuex compiler to evaluate cue files",
		Example: `vela cuex`,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
	}

	cmd.AddCommand(newCuexEvalCommand(c, ioStreams))

	return cmd
}

func newCuexEvalCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var (
		expression string
		out        string
	)

	cmd := &cobra.Command{
		Use:     "eval <cue file>",
		Long:    "eval evaluates, validates, and prints a configuration.",
		Example: `vela cuex eval`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("a file is needed")
			}

			bytes, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			value, err := defaultCuexCompiler.CompileString(cmd.Context(), string(bytes))
			if err != nil {
				return fmt.Errorf("failed to evaluate cue file: %w", err)
			}

			if expression != "" {
				value = value.LookupPath(cue.ParsePath(expression))
				if err := value.Validate(); err != nil {
					return fmt.Errorf("failed to parse: %w", err)
				}
			}

			switch out {
			case "", "cue":
				synOpts := []cue.Option{
					cue.Final(), // for backwards compatibility
					cue.Definitions(true),
					cue.Docs(true),
					cue.Attributes(true),
					cue.Optional(true),
					cue.Definitions(true),
					cue.DisallowCycles(true),
					cue.InlineImports(true),
				}

				// Keep for legacy reasons. Note that `cue eval` is to be deprecated by
				// `cue` eventually.
				opts := []format.Option{
					format.UseSpaces(4),
					format.TabIndent(false),
					format.Simplify(),
				}

				//root := value.Eval()
				b, err := format.Node(ToFile(value.Syntax(synOpts...)), opts...)
				if err != nil {
					return fmt.Errorf("failed to format output: %w", err)
				}
				_, err = ioStreams.Out.Write(b)
			case "json":
				d := json.NewEncoder(ioStreams.Out)
				d.SetIndent("", "    ")
				d.SetEscapeHTML(true)
				err = d.Encode(value)
			case "yaml":
				str, err := yaml.Marshal(value)
				if err != nil {
					return err
				}
				fmt.Fprintln(ioStreams.Out, str)
			default:
				return fmt.Errorf("unknown out format: %s", out)
			}

			return err
		},
	}

	cmd.SetOut(ioStreams.Out)
	cmd.Flags().StringVarP(&expression, "expression", "e", "", "evaluate this expression only")
	cmd.Flags().StringVar(&out, "out", "", "output format[cue|yaml|json]")

	return cmd
}

// ToFile converts an expression to a file.
//
// Adjusts the spacing of x when needed.
func ToFile(n ast.Node) *ast.File {
	switch x := n.(type) {
	case nil:
		return nil
	case *ast.StructLit:
		return &ast.File{Decls: x.Elts}
	case ast.Expr:
		ast.SetRelPos(x, cuetoken.NoSpace)
		return &ast.File{Decls: []ast.Decl{&ast.EmbedDecl{Expr: x}}}
	case *ast.File:
		return x
	default:
		panic(fmt.Sprintf("Unsupported node type %T", x))
	}
}
