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

package model

import (
	"testing"

	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
)

type mockListener struct{}

func (l *mockListener) StackPop(_ View, _ View) {}
func (l *mockListener) StackPush(_ View)        {}

type mockComponent struct {
	tview.Primitive
}

func (c *mockComponent) Init() {}
func (c *mockComponent) Name() string {
	return ""
}
func (c *mockComponent) Start() {}
func (c *mockComponent) Stop()  {}
func (c *mockComponent) Hint() []MenuHint {
	return []MenuHint{}
}

func TestStack(t *testing.T) {
	stack := NewStack()
	assert.Equal(t, stack.Empty(), true)
	l := &mockListener{}
	stack.AddListener(l)
	c := &mockComponent{}
	stack.PushView(c)
	assert.Equal(t, stack.IsLastView(), true)
	assert.Equal(t, stack.TopView(), c)
	stack.PopView()
	assert.Equal(t, stack.Empty(), true)
	stack.RemoveListener(l)
	assert.Equal(t, len(stack.listeners), 0)
}
