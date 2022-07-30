package model

import "sync"

const (
	stackPush = iota
	StackPop
)

type ResourceListener interface {
	StackPop(Component, Component)
	StackPush(Component)
}

type Stack struct {
	components []Component
	listeners  []ResourceListener
	mx         sync.RWMutex
}

func NewStack() *Stack {
	return &Stack{
		components: make([]Component, 0),
		listeners:  make([]ResourceListener, 0),
	}
}

func (s *Stack) AddListener(listener ResourceListener) {
	s.listeners = append(s.listeners, listener)
}

func (s *Stack) RemoveListener(listener ResourceListener) {
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

func (s *Stack) TopComponent() Component {
	if s.Empty() {
		return nil
	}
	component := s.components[len(s.components)-1]
	return component
}

func (s *Stack) IsLastComponent() bool {
	return len(s.components) == 1
}

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

func (s *Stack) PushComponent(component Component) {
	if top := s.TopComponent(); top != nil {
		top.Stop()
	}

	s.mx.Lock()
	s.components = append(s.components, component)
	s.mx.Unlock()

	s.notifyListener(stackPush, component)
}

func (s *Stack) Empty() bool {
	return len(s.components) == 0
}

func (s *Stack) notifyListener(action int, component Component) {
	for _, listener := range s.listeners {
		switch action {
		case stackPush:
			listener.StackPush(component)
		case StackPop:
			listener.StackPop(component, s.TopComponent())
		}
	}
}
