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
	"context"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

const (
	// RefreshDelay is refresh delay
	RefreshDelay       = 10
	resourceReqTimeout = 3
)

// ResourceView is the interface to abstract resource view
type ResourceView interface {
	model.View
	InitView(ctx context.Context, app *App)
	Refresh(event *tcell.EventKey) *tcell.EventKey
	Update(timeoutCancel func())
	BuildHeader()
	BuildBody()
}

// ResourceViewMap is a map from resource name to resource view
var ResourceViewMap = map[string]ResourceView{
	"app":       new(ApplicationView),
	"cluster":   new(ClusterView),
	"resource":  new(ManagedResourceView),
	"ns":        new(NamespaceView),
	"cns":       new(ClusterNamespaceView),
	"pod":       new(PodView),
	"container": new(ContainerView),
}

// CommonResourceView is an abstract of resource view
type CommonResourceView struct {
	*component.Table
	app        *App
	cancelFunc func()
}

// NewCommonView return a new common view
func NewCommonView(app *App) *CommonResourceView {
	resourceView := &CommonResourceView{
		Table:      component.NewTable(app.config.Theme),
		app:        app,
		cancelFunc: func() {},
	}
	return resourceView
}

// Init the common resource view
func (v *CommonResourceView) Init() {
	v.Table.Init()
	v.SetBorder(true)
	v.SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.SetSelectable(true, false)
	v.bindKeys()
	v.app.SetFocus(v)
}

// Name return the name of common view
func (v *CommonResourceView) Name() string {
	return "Resource"
}

// BuildHeader render the header of table
func (v *CommonResourceView) BuildHeader(header []string) {
	for i := 0; i < len(header); i++ {
		c := tview.NewTableCell(header[i])
		c.SetTextColor(v.app.config.Theme.Table.Header.Color())
		c.SetExpansion(3)
		v.SetCell(0, i, c)
	}
}

// BuildBody render the body of table
func (v *CommonResourceView) BuildBody(body [][]string) {
	rowNum := len(body)
	for i := 0; i < rowNum; i++ {
		columnNum := len(body[i])
		for j := 0; j < columnNum; j++ {
			c := tview.NewTableCell(body[i][j])
			c.SetTextColor(v.app.config.Theme.Table.Body.Color())
			c.SetExpansion(3)
			v.SetCell(i+1, j, c)
		}
	}
}

// Stop the refresh goroutine and clear the table content
func (v *CommonResourceView) Stop() {
	v.Table.Stop()
	v.cancelFunc()
}

// Refresh the base resource view
func (v *CommonResourceView) Refresh(clear bool, update func(timeoutCancel func())) {
	if clear {
		v.Clear()
	}
	updateWithTimeout := func() {
		ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*resourceReqTimeout)
		defer cancelFunc()
		go update(cancelFunc)

		select {
		case <-time.After(time.Second * resourceReqTimeout): // timeout
		case <-ctx.Done(): // success
		}
	}

	v.app.QueueUpdateDraw(updateWithTimeout)
}

// AutoRefresh will refresh the view in every RefreshDelay delay
func (v *CommonResourceView) AutoRefresh(update func(timeoutCancel func())) {
	var ctx context.Context
	ctx, v.cancelFunc = context.WithCancel(context.Background())
	go func() {
		for {
			time.Sleep(RefreshDelay * time.Second)
			select {
			case <-ctx.Done():
				return
			default:
				v.Refresh(true, update)
			}
		}
	}()
}

func (v *CommonResourceView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyESC})
	v.Actions().Add(model.KeyActions{
		component.KeyQ:    model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
		tcell.KeyCtrlT:    model.KeyAction{Description: "Switch Theme", Action: v.app.SwitchTheme, Visible: true, Shared: true},
	})
}
