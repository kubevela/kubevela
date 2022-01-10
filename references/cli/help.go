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
	"sort"

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/spf13/cobra"
)

// NewHelpCommand get any command help
func NewHelpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "help [command] ",
		DisableFlagsInUseLine: true,
		Short:                 "Help about any command",
		Run:                   RunHelp,
	}
	return cmd
}

// RunHelp exec help [command]
func RunHelp(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		allCommands := cmd.Root().Commands()
		// print error message at first, since it can contain suggestions
		cmd.Printf("A Highly Extensible Platform Engine based on Kubernetes and Open Application Model.\n\nUsage:\n  vela [flags]\n  vela [command]\n\nAvailable Commands:\n\n")
		PrintHelpByTag(cmd, allCommands, types.TypeStart)
		PrintHelpByTag(cmd, allCommands, types.TypeApp)
		PrintHelpByTag(cmd, allCommands, types.TypeExtension)
		PrintHelpByTag(cmd, allCommands, types.TypeSystem)
	} else {
		foundCmd, _, err := cmd.Root().Find(args)
		if foundCmd != nil && err == nil {
			foundCmd.HelpFunc()(cmd, args)
		}
	}
}

// Printable is a struct for print help
type Printable struct {
	Order string
	use   string
	Long  string
}

// PrintList is a list of Printable
type PrintList []Printable

func (p PrintList) Len() int {
	return len(p)
}
func (p PrintList) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
func (p PrintList) Less(i, j int) bool {
	return p[i].Order > p[j].Order
}

// PrintHelpByTag print custom defined help message
func PrintHelpByTag(cmd *cobra.Command, all []*cobra.Command, tag string) {
	table := newUITable()
	var pl PrintList
	for _, c := range all {
		if val, ok := c.Annotations[types.TagCommandType]; ok && val == tag {
			pl = append(pl, Printable{Order: c.Annotations[types.TagCommandOrder], use: c.Use, Long: c.Long})
		}
	}
	if len(all) == 0 {
		return
	}
	cmd.Println("  " + tag + ":")
	cmd.Println()

	sort.Sort(pl)

	for _, v := range pl {
		table.AddRow("    "+v.use, v.Long)
	}
	cmd.Println(table.String())
	cmd.Println()
}
