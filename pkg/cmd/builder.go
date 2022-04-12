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

package cmd

import (
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/term"
)

// Builder build command with factory
type Builder struct {
	cmd *cobra.Command
	f   Factory
}

// NamespaceFlagConfig config for namespace flag in cmd
type NamespaceFlagConfig struct {
	completion bool
	usage      string
	loadEnv    bool
}

// NamespaceFlagOption the option for configuring namespace flag in cmd
type NamespaceFlagOption interface {
	ApplyToNamespaceFlagOptions(*NamespaceFlagConfig)
}

func newNamespaceFlagOptions(options ...NamespaceFlagOption) NamespaceFlagConfig {
	cfg := NamespaceFlagConfig{
		completion: true,
		usage:      usageNamespace,
		loadEnv:    true,
	}
	for _, option := range options {
		option.ApplyToNamespaceFlagOptions(&cfg)
	}
	return cfg
}

// NamespaceFlagNoCompletionOption disable auto-completion for namespace flag
type NamespaceFlagNoCompletionOption struct{}

// ApplyToNamespaceFlagOptions .
func (option NamespaceFlagNoCompletionOption) ApplyToNamespaceFlagOptions(cfg *NamespaceFlagConfig) {
	cfg.completion = false
}

// NamespaceFlagUsageOption the usage description for namespace flag
type NamespaceFlagUsageOption string

// ApplyToNamespaceFlagOptions .
func (option NamespaceFlagUsageOption) ApplyToNamespaceFlagOptions(cfg *NamespaceFlagConfig) {
	cfg.usage = string(option)
}

// NamespaceFlagDisableEnvOption disable loading namespace from env
type NamespaceFlagDisableEnvOption struct{}

// ApplyToNamespaceFlagOptions .
func (option NamespaceFlagDisableEnvOption) ApplyToNamespaceFlagOptions(cfg *NamespaceFlagConfig) {
	cfg.loadEnv = false
}

// WithNamespaceFlag add namespace flag to the command, by default, it will also add env flag to the command
func (builder *Builder) WithNamespaceFlag(options ...NamespaceFlagOption) *Builder {
	cfg := newNamespaceFlagOptions(options...)
	builder.cmd.Flags().StringP(flagNamespace, "n", "", cfg.usage)
	if cfg.completion {
		cmdutil.CheckErr(builder.cmd.RegisterFlagCompletionFunc(
			flagNamespace,
			func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return GetNamespacesForCompletion(cmd.Context(), builder.f, toComplete)
			}))
	}
	if cfg.loadEnv {
		return builder.WithEnvFlag()
	}
	return builder
}

// WithEnvFlag add env flag to the command
func (builder *Builder) WithEnvFlag() *Builder {
	builder.cmd.PersistentFlags().StringP(flagEnv, "e", "", usageEnv)
	return builder
}

// WithResponsiveWriter format the command outputs
func (builder *Builder) WithResponsiveWriter() *Builder {
	builder.cmd.SetOut(term.NewResponsiveWriter(builder.cmd.OutOrStdout()))
	return builder
}

// Build construct the command
func (builder *Builder) Build() *cobra.Command {
	return builder.cmd
}

// NewCommandBuilder builder for command
func NewCommandBuilder(f Factory, cmd *cobra.Command) *Builder {
	return &Builder{cmd: cmd, f: f}
}
