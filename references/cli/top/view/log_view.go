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
	"fmt"
	"io"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// LogView is the log view, this view display log of pod
type LogView struct {
	*tview.TextView
	app        *App
	actions    model.KeyActions
	ctx        context.Context
	writer     io.Writer
	cancelFunc func()
}

var (
	logViewInstance = new(LogView)
)

// NewLogView return a new log view
func NewLogView(ctx context.Context, app *App) model.View {
	logViewInstance.ctx = ctx
	if logViewInstance.TextView == nil {
		logViewInstance.TextView = tview.NewTextView()
		logViewInstance.app = app
		logViewInstance.actions = make(model.KeyActions)
		logViewInstance.writer = tview.ANSIWriter(logViewInstance.TextView)
	}
	return logViewInstance
}

// Init the log view
func (v *LogView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetDynamicColors(true)
	v.SetBorder(true)
	v.SetBorderAttributes(tcell.AttrItalic)
	v.SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.SetTitle(title)
	v.SetBorderPadding(1, 1, 2, 2)
	v.bindKeys()
	v.SetInputCapture(v.keyboard)
}

// Start the log view
func (v *LogView) Start() {
	var ctx context.Context
	ctx, v.cancelFunc = context.WithCancel(context.Background())

	cluster := v.ctx.Value(&model.CtxKeyCluster).(string)
	pod := v.ctx.Value(&model.CtxKeyPod).(string)
	namespace := v.ctx.Value(&model.CtxKeyNamespace).(string)

	logC, err := model.PrintLogOfPod(ctx, v.app.config.RestConfig, cluster, namespace, pod, "")
	if err != nil {
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case line := <-logC:
				v.app.QueueUpdateDraw(func() {
					_, _ = v.writer.Write([]byte(line))
				})
			default:
				time.Sleep(time.Second)
			}
		}
	}()
}

// Stop the log view
func (v *LogView) Stop() {
	v.cancelFunc()
	v.Clear()
}

// Name return the name of log view
func (v *LogView) Name() string {
	return "Log"
}

// Hint return the menu hints of log view
func (v *LogView) Hint() []model.MenuHint {
	return v.actions.Hint()
}

func (v *LogView) keyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	if key == tcell.KeyUp || key == tcell.KeyDown {
		return event
	}
	if a, ok := v.actions[component.StandardizeKey(event)]; ok {
		return a.Action(event)
	}
	return event
}

func (v *LogView) bindKeys() {
	v.actions.Delete([]tcell.Key{tcell.KeyEnter})
	v.actions.Add(model.KeyActions{
		component.KeyQ:    model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
