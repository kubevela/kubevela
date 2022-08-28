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
	"log"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ApplicationView is the application view, this view display info of application of KubeVela
type ApplicationView struct {
	*ResourceView
	ctx context.Context
}

// NewApplicationView return a new application view
func NewApplicationView(ctx context.Context, app *App) model.Component {
	v := &ApplicationView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init the application view
func (v *ApplicationView) Init() {
	// set title of view
	title := fmt.Sprintf("[ %s ]", v.Title())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)
	// init view
	resourceList := v.ListApplications()
	v.ResourceView.Init(resourceList)

	v.ColorizeStatusText(len(resourceList.Body()))
	v.bindKeys()
}

// ListApplications list all applications
func (v *ApplicationView) ListApplications() model.ResourceList {
	list, err := model.ListApplications(v.ctx, v.app.client)
	if err != nil {
		log.Println(err)
	}
	return list
}

// ColorizeStatusText colorize the status column text
func (v *ApplicationView) ColorizeStatusText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		status := v.Table.GetCell(i, 2).Text
		switch common.ApplicationPhase(status) {
		case common.ApplicationRendering, common.ApplicationStarting:
			status = config.ApplicationStartingAndRenderingPhaseColor + status
		case common.ApplicationWorkflowSuspending:
			status = config.ApplicationWorkflowSuspendingPhaseColor + status
		case common.ApplicationWorkflowTerminated:
			status = config.ApplicationWorkflowTerminatedPhaseColor + status
		case common.ApplicationRunning:
			status = config.ApplicationRunningPhaseColor + status
		default:
		}
		v.Table.GetCell(i, 2).SetText(status)
	}
}

// Title return table title of application view
func (v *ApplicationView) Title() string {
	namespace := v.ctx.Value(&model.CtxKeyNamespace).(string)
	if namespace == "" {
		namespace = "all"
	}
	return fmt.Sprintf("Application"+" (%s)", namespace)
}

// Name return application view name
func (v *ApplicationView) Name() string {
	return "Application"
}

// Hint return key action menu hints of the application view
func (v *ApplicationView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *ApplicationView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Enter", Action: v.managedResourceView, Visible: true, Shared: true},
		component.KeyN:    model.KeyAction{Description: "Select Namespace", Action: v.namespaceView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

func (v *ApplicationView) managedResourceView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyAppName, name)
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyNamespace, namespace)

	v.app.command.run(v.ctx, "resource")
	return event
}

func (v *ApplicationView) namespaceView(event *tcell.EventKey) *tcell.EventKey {
	v.app.content.Clear()
	v.app.command.run(v.ctx, "ns")
	return event
}
