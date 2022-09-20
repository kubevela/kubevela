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

type mockListener struct {
	data int
}

func (l *mockListener) StackPop(_ View, _ View) {
	l.data--
}
func (l *mockListener) StackPush(_, _ View) {
	l.data++
}

type mockView struct {
	tview.Primitive
}

func (c *mockView) Init() {}
func (c *mockView) Name() string {
	return ""
}
func (c *mockView) Start() {}
func (c *mockView) Stop()  {}
func (c *mockView) Hint() []MenuHint {
	return []MenuHint{}
}

func TestStack(t *testing.T) {
	stack := NewStack()
	assert.Equal(t, stack.Empty(), true)
	l := &mockListener{}
	stack.AddListener(l)
	assert.Equal(t, len(stack.listeners), 1)

	c := &mockView{}
	stack.PushView(c)
	assert.Equal(t, l.data, 1)
	assert.Equal(t, stack.IsLastView(), true)
	assert.Equal(t, stack.TopView(), c)

	stack.PopView()
	assert.Equal(t, l.data, 0)
	assert.Equal(t, stack.Empty(), true)

	stack.PushView(c)
	stack.Clear()
	assert.Equal(t, stack.Empty(), true)

	stack.RemoveListener(l)
	assert.Equal(t, len(stack.listeners), 0)
}
