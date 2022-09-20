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
	StackPop(old, new View)
	// StackPush push a new component
	StackPush(old, new View)
}

// Stack is a stack to store components and notify listeners of main view of app
type Stack struct {
	views     []View
	listeners []ViewListener
	mutex     sync.RWMutex
}

// NewStack return a new stack instance
func NewStack() *Stack {
	return &Stack{
		views:     make([]View, 0),
		listeners: make([]ViewListener, 0),
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

// TopView return top view of stack
func (s *Stack) TopView() View {
	if s.Empty() {
		return nil
	}
	return s.views[len(s.views)-1]
}

// IsLastView check whether stack only have one view now
func (s *Stack) IsLastView() bool {
	return len(s.views) == 1
}

// PopView pop a view from stack
func (s *Stack) PopView() {
	if s.Empty() {
		return
	}

	s.mutex.Lock()
	removeComponent := s.views[len(s.views)-1]
	s.views = s.views[:len(s.views)-1]
	s.mutex.Unlock()

	s.notifyListener(StackPop, removeComponent, s.TopView())
}

// PushView add a new view to stack
func (s *Stack) PushView(component View) {
	old := s.TopView()
	s.mutex.Lock()
	s.views = append(s.views, component)
	s.mutex.Unlock()

	s.notifyListener(stackPush, old, component)
}

// Empty return whether stack is empty
func (s *Stack) Empty() bool {
	return len(s.views) == 0
}

// Clear out the stack
func (s *Stack) Clear() {
	for !s.Empty() {
		s.PopView()
	}
}

func (s *Stack) notifyListener(action int, old, new View) {
	for _, listener := range s.listeners {
		switch action {
		case stackPush:
			listener.StackPush(old, new)
		case StackPop:
			listener.StackPop(old, new)
		}
	}
}
