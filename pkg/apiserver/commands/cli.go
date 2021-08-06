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

package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/version"
)

// CLI for apiserver
type CLI struct {
	rootCmd *cobra.Command
}

// NewCLI create new CLI for apiserver
func NewCLI(name, desc string) *CLI {
	a := &CLI{
		rootCmd: &cobra.Command{
			Use:           name,
			Short:         desc,
			SilenceErrors: true,
		},
	}
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the information of current binary.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("KubeVela information:", "version", version.VelaVersion, ", gitRevision", version.GitRevision)
		},
	}
	a.rootCmd.AddCommand(versionCmd)
	a.setGlobalFlags()
	return a
}

func (c *CLI) setGlobalFlags() {
	// set global flags here
}

// AddCommands apiserver add command function
func (c *CLI) AddCommands(cmds ...*cobra.Command) {
	for _, cmd := range cmds {
		c.rootCmd.AddCommand(cmd)
	}
}

// Run apiserver run function
func (c *CLI) Run() error {
	return c.rootCmd.Execute()
}
