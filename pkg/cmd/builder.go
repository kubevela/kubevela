/*
Copyright 2022 The KubeVela Authors.

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
	"k8s.io/kubectl/pkg/util/term"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/utils/util"
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

// ClusterFlagConfig config for cluster flag in cmd
type ClusterFlagConfig struct {
	completion        bool
	usage             string
	disableSliceInput bool
}

// ClusterFlagOption the option for configuring cluster flag
type ClusterFlagOption interface {
	ApplyToClusterFlagOptions(*ClusterFlagConfig)
}

func newClusterFlagOptions(options ...ClusterFlagOption) ClusterFlagConfig {
	cfg := ClusterFlagConfig{
		completion:        true,
		usage:             usageCluster,
		disableSliceInput: false,
	}
	for _, option := range options {
		option.ApplyToClusterFlagOptions(&cfg)
	}
	return cfg
}

// FlagNoCompletionOption disable auto-completion for flag
type FlagNoCompletionOption struct{}

// ApplyToNamespaceFlagOptions .
func (option FlagNoCompletionOption) ApplyToNamespaceFlagOptions(cfg *NamespaceFlagConfig) {
	cfg.completion = false
}

// ApplyToClusterFlagOptions .
func (option FlagNoCompletionOption) ApplyToClusterFlagOptions(cfg *ClusterFlagConfig) {
	cfg.completion = false
}

// UsageOption the usage description for flag
type UsageOption string

// ApplyToNamespaceFlagOptions .
func (option UsageOption) ApplyToNamespaceFlagOptions(cfg *NamespaceFlagConfig) {
	cfg.usage = string(option)
}

// ApplyToClusterFlagOptions .
func (option UsageOption) ApplyToClusterFlagOptions(cfg *ClusterFlagConfig) {
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

// ClusterFlagDisableSliceInputOption set the cluster flag to allow multiple input
type ClusterFlagDisableSliceInputOption struct{}

// ApplyToClusterFlagOptions .
func (option ClusterFlagDisableSliceInputOption) ApplyToClusterFlagOptions(cfg *ClusterFlagConfig) {
	cfg.disableSliceInput = true
}

// WithClusterFlag add cluster flag to the command
func (builder *Builder) WithClusterFlag(options ...ClusterFlagOption) *Builder {
	cfg := newClusterFlagOptions(options...)
	if cfg.disableSliceInput {
		builder.cmd.Flags().StringP(flagCluster, "c", types.ClusterLocalName, cfg.usage)
	} else {
		builder.cmd.Flags().StringSliceP(flagCluster, "c", []string{types.ClusterLocalName}, cfg.usage)
	}
	if cfg.completion {
		cmdutil.CheckErr(builder.cmd.RegisterFlagCompletionFunc(
			flagCluster,
			func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return GetClustersForCompletion(cmd.Context(), builder.f, toComplete)
			}))
	}
	return builder
}

// WithStreams set the in/out/err streams for the command
func (builder *Builder) WithStreams(streams util.IOStreams) *Builder {
	builder.cmd.SetIn(streams.In)
	builder.cmd.SetOut(streams.Out)
	builder.cmd.SetErr(streams.ErrOut)
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
