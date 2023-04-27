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
	"math"
	"strconv"

	"github.com/kubevela/pkg/util/slices"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"

	"github.com/oam-dev/kubevela/apis/types"
)

// NewHelpCommand get any command help
func NewHelpCommand(order string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "help",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Help about any command."),
		Example:               "help [command] | STRING_TO_SEARCH",
		Run:                   RunHelp,
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeAuxiliary,
			types.TagCommandOrder: order,
		},
	}
	return cmd
}

// RunHelp exec help [command]
func RunHelp(cmd *cobra.Command, args []string) {
	runHelp(cmd, cmd.Root().Commands(), args)
}

func runHelp(cmd *cobra.Command, allCommands []*cobra.Command, args []string) {
	if len(args) == 0 {
		cmd.Printf("A Highly Extensible Platform Engine based on Kubernetes and Open Application Model.\n\n")
		for _, t := range []string{types.TypeStart, types.TypeApp, types.TypeCD, types.TypePlatform, types.TypeExtension, types.TypeSystem, types.TypeAuxiliary} {
			PrintHelpByTag(cmd, allCommands, t)
		}
		cmd.Println("Flags:")
		cmd.Println("  -h, --help   help for vela")
		cmd.Println()
		cmd.Println(`Use "vela [command] --help" for more information about a command.`)
	} else {
		foundCmd, _, err := cmd.Root().Find(args)
		if foundCmd != nil && err == nil {
			foundCmd.HelpFunc()(foundCmd, args)
		}
	}
}

// Printable is a struct for print help
type Printable struct {
	Order int64
	Use   string
	Desc  string
}

// NewPrintable create printable object
func NewPrintable(c *cobra.Command, desc string) Printable {
	order, err := strconv.ParseInt(c.Annotations[types.TagCommandOrder], 10, 64)
	if err != nil {
		order = math.MaxInt
	}
	return Printable{Order: order, Use: c.Use, Desc: desc}
}

// PrintHelpByTag print custom defined help message
func PrintHelpByTag(cmd *cobra.Command, all []*cobra.Command, tag string) {
	table := newUITable()
	table.MaxColWidth = 80
	var pl []Printable
	for _, c := range all {
		if c.Hidden || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if val, ok := c.Annotations[types.TagCommandType]; ok && val == tag {
			pl = append(pl, NewPrintable(c, c.Short))
		}
	}
	if len(all) == 0 {
		return
	}
	slices.Sort(pl, func(i, j Printable) bool { return i.Order < j.Order })
	cmd.Println(tag + ":")
	for _, v := range pl {
		table.AddRow(fmt.Sprintf("  %-15s", v.Use), v.Desc)
	}
	cmd.Println(table.String())
	cmd.Println()
}
