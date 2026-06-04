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
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestPerformanceConfig_Validate(t *testing.T) {
	cfg := NewPerformanceConfig()
	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestPerformanceConfig_AddFlags(t *testing.T) {
	cfg := NewPerformanceConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)
	err := fs.Parse([]string{"--perf-enabled=true"})
	assert.NoError(t, err)
	assert.True(t, cfg.PerfEnabled, "PerfEnabled should be bound to --perf-enabled")
}
