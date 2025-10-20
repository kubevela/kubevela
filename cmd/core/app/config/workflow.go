/*
Copyright 2025 The KubeVela Authors.

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

package config

import (
	"github.com/spf13/pflag"

	wfTypes "github.com/kubevela/workflow/pkg/types"
)

// WorkflowConfig contains workflow engine configuration.
type WorkflowConfig struct {
	MaxWaitBackoffTime     int
	MaxFailedBackoffTime   int
	MaxStepErrorRetryTimes int
}

// NewWorkflowConfig creates a new WorkflowConfig with defaults.
func NewWorkflowConfig() *WorkflowConfig {
	return &WorkflowConfig{
		MaxWaitBackoffTime:     60,
		MaxFailedBackoffTime:   300,
		MaxStepErrorRetryTimes: 10,
	}
}

// AddFlags registers workflow configuration flags.
func (c *WorkflowConfig) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.MaxWaitBackoffTime,
		"max-workflow-wait-backoff-time",
		c.MaxWaitBackoffTime,
		"Set the max workflow wait backoff time, default is 60")
	fs.IntVar(&c.MaxFailedBackoffTime,
		"max-workflow-failed-backoff-time",
		c.MaxFailedBackoffTime,
		"Set the max workflow failed backoff time, default is 300")
	fs.IntVar(&c.MaxStepErrorRetryTimes,
		"max-workflow-step-error-retry-times",
		c.MaxStepErrorRetryTimes,
		"Set the max workflow step error retry times, default is 10")
}

// SyncToWorkflowGlobals syncs the parsed configuration values to workflow package global variables.
// This should be called after flag parsing to ensure the workflow engine uses the configured values.
//
// NOTE: This method exists for backward compatibility with legacy code that depends on global
// variables in the wfTypes package. The long-term goal should be to refactor the workflow
// package to accept configuration via dependency injection rather than globals.
//
// The flow is: CLI flags -> WorkflowConfig struct fields -> wfTypes globals (via this method)
func (c *WorkflowConfig) SyncToWorkflowGlobals() {
	wfTypes.MaxWorkflowWaitBackoffTime = c.MaxWaitBackoffTime
	wfTypes.MaxWorkflowFailedBackoffTime = c.MaxFailedBackoffTime
	wfTypes.MaxWorkflowStepErrorRetryTimes = c.MaxStepErrorRetryTimes
}
