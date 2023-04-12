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

package resourcekeeper

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// configType Generic constraints for config
type configType interface {
	*gcConfig | *dispatchConfig | *deleteConfig
}

// IContextInterface public method for gcContext|deleteContext|dispatchContext
type IContextInterface[O any, C configType] interface {
	WithWorkflowPhase(phase common.ApplicationPhase) *IContext[O, C]
	WithOption(opt O) *IContext[O, C]
	WithOptions(opts []O) *IContext[O, C]
	WithActiveOption(active bool, opt O) *IContext[O, C]
	CleanUpOptions() *IContext[O, C]
	GetWorkflowPhase() common.ApplicationPhase
	SetConfigHandler(func(...O) C) *IContext[O, C]
	GetConfig() C
}

// IContext private field for gcContext|deleteContext|dispatchContext
type IContext[O any, C configType] struct {
	workflowPhase common.ApplicationPhase
	options       []O
	configHandler func(...O) C
}

// WithOption append options
func (c *IContext[O, C]) WithOption(opt O) *IContext[O, C] {
	c.options = append(c.options, opt)
	return c
}

// WithOptions append options
func (c *IContext[O, C]) WithOptions(opts []O) *IContext[O, C] {
	c.options = append(c.options, opts...)
	return c
}

// CleanUpOptions clean up options
func (c *IContext[O, C]) CleanUpOptions() *IContext[O, C] {
	c.options = []O{}
	return c
}

// WithWorkflowPhase set workflow phase
func (c *IContext[O, C]) WithWorkflowPhase(phase common.ApplicationPhase) *IContext[O, C] {
	c.workflowPhase = phase
	return c
}

// WithActiveOption active if set, append to options
func (c *IContext[O, C]) WithActiveOption(active bool, opt O) *IContext[O, C] {
	if active {
		c.options = append(c.options, opt)
	}
	return c
}

// GetWorkflowPhase get workflow phase
func (c *IContext[O, C]) GetWorkflowPhase() common.ApplicationPhase {
	return c.workflowPhase
}

// SetConfigHandler  set configHandler
func (c *IContext[O, C]) SetConfigHandler(configHandler func(...O) C) *IContext[O, C] {
	c.configHandler = configHandler
	return c
}

// GetConfig  get config by execute configHandler
func (c *IContext[O, C]) GetConfig() C {
	return c.configHandler(c.options...)
}

// GCContext implement from IContext
type GCContext[O GCOption, C *gcConfig] struct {
	IContext[O, C]
}

// NewGCContext create gc context, return IContextInterface
func NewGCContext() IContextInterface[GCOption, *gcConfig] {
	c := &GCContext[GCOption, *gcConfig]{}
	c.SetConfigHandler(newGCConfig)
	return c
}

// DispatchContext implement from IContext
type DispatchContext[O DispatchOption, C *dispatchConfig] struct {
	IContext[O, C]
}

// NewDispatchContext create dispatch context, return IContextInterface
func NewDispatchContext() IContextInterface[DispatchOption, *dispatchConfig] {
	c := &DispatchContext[DispatchOption, *dispatchConfig]{}
	c.SetConfigHandler(newDispatchConfig)
	return c
}

// DeleteContext implement from IContext
type DeleteContext[O DeleteOption, C *deleteConfig] struct {
	IContext[O, C]
}

// NewDeleteContext create delete context, return IContextInterface
func NewDeleteContext() IContextInterface[DeleteOption, *deleteConfig] {
	c := &DeleteContext[DeleteOption, *deleteConfig]{}
	c.SetConfigHandler(newDeleteConfig)
	return c
}
