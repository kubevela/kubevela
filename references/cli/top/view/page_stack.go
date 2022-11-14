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

package view

import (
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// PageStack store views of app's main view, and it's a high level packing of "component.Pages"
type PageStack struct {
	*component.Pages
	app *App
}

// NewPageStack returns a new page stack.
func NewPageStack(app *App) *PageStack {
	ps := &PageStack{
		Pages: component.NewPages(),
		app:   app,
	}
	return ps
}

// Init the pageStack
func (ps *PageStack) Init() {
	ps.Stack.AddListener(ps)
}

// StackPop change itself when accept "pop" notify from app's main view
func (ps *PageStack) StackPop(old, new model.View) {
	if new == nil {
		return
	}
	ps.app.QueueUpdateDraw(new.Start)
	ps.app.SetFocus(new)
	go old.Stop()
}

// StackPush change itself when accept "pop" notify from app's main view
func (ps *PageStack) StackPush(old, new model.View) {
	ps.app.QueueUpdateDraw(new.Start)
	ps.app.SetFocus(new)
	if old != nil {
		go old.Stop()
	}
}
