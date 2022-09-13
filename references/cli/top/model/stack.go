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

import "sync"

const (
	// stackPush is the "push" notify type
	stackPush = iota
	// StackPop is the "pop" notify type
	StackPop
)

// ViewListener listen notify from the main view of app and render itself again
type ViewListener interface {
	// StackPop pop old component and render component
	StackPop(View, View)
	// StackPush push a new component
	StackPush(View)
}

// Stack is a stack to store components and notify listeners of main view of app
type Stack struct {
	components []View
	listeners  []ViewListener
	mx         sync.RWMutex
}

// NewStack return a new stack instance
func NewStack() *Stack {
	return &Stack{
		components: make([]View, 0),
		listeners:  make([]ViewListener, 0),
	}
}

// AddListener add a new resource listener
func (s *Stack) AddListener(listener ViewListener) {
	s.listeners = append(s.listeners, listener)
}

// RemoveListener remove the aim resource listener
func (s *Stack) RemoveListener(listener ViewListener) {
	aimIndex := -1
	for index, item := range s.listeners {
		if item == listener {
			aimIndex = index
		}
	}
	if aimIndex == -1 {
		return
	}
	s.listeners = append(s.listeners[:aimIndex], s.listeners[aimIndex+1:]...)
}

// TopComponent return top component of stack
func (s *Stack) TopComponent() View {
	if s.Empty() {
		return nil
	}
	return s.components[len(s.components)-1]
}

// IsLastComponent check whether stack only have one component now
func (s *Stack) IsLastComponent() bool {
	return len(s.components) == 1
}

// PopComponent pop a component from stack
func (s *Stack) PopComponent() {
	if s.Empty() {
		return
	}

	s.mx.Lock()
	removeComponent := s.components[len(s.components)-1]
	s.components = s.components[:len(s.components)-1]
	s.mx.Unlock()

	s.notifyListener(StackPop, removeComponent)
}

// PushComponent add a new component to stack
func (s *Stack) PushComponent(component View) {
	if top := s.TopComponent(); top != nil {
		top.Stop()
	}

	s.mx.Lock()
	s.components = append(s.components, component)
	s.mx.Unlock()

	s.notifyListener(stackPush, component)
}

// Empty return whether stack is empty
func (s *Stack) Empty() bool {
	return len(s.components) == 0
}

// Clear out the stack
func (s *Stack) Clear() {
	for !s.Empty() {
		s.PopComponent()
	}
}

func (s *Stack) notifyListener(action int, component View) {
	for _, listener := range s.listeners {
		switch action {
		case stackPush:
			listener.StackPush(component)
		case StackPop:
			listener.StackPop(component, s.TopComponent())
		}
	}
}
