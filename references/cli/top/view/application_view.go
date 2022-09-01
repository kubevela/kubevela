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

// Name return application view name
func (v *ApplicationView) Name() string {
	return "Application"
}

// Init the application view
func (v *ApplicationView) Init() {
	v.ResourceView.Init()
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Title()))
	v.BuildHeader()
	v.bindKeys()
}

// Start the application view
func (v *ApplicationView) Start() {
	v.Update()
}

// Stop the application view
func (v *ApplicationView) Stop() {
	v.Table.Stop()
}

// Hint return key action menu hints of the application view
func (v *ApplicationView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// InitView return a new application view
func (v *ApplicationView) InitView(ctx context.Context, app *App) {
	if v.ResourceView == nil {
		v.ResourceView = NewResourceView(app)
		v.ctx = ctx
	} else {
		v.ctx = ctx
	}
}

// Update refresh the content of body of view
func (v *ApplicationView) Update() {
	v.BuildBody()
}

// BuildHeader render the header of table
func (v *ApplicationView) BuildHeader() {
	header := []string{"Name", "Namespace", "Phase", "CreateTime"}
	v.ResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *ApplicationView) BuildBody() {
	apps, err := model.ListApplications(v.ctx, v.app.client)
	if err != nil {
		return
	}
	appInfos := apps.ToTableBody()
	v.ResourceView.BuildBody(appInfos)
	rowNum := len(appInfos)
	v.ColorizeStatusText(rowNum)
}

// ColorizeStatusText colorize the status column text
func (v *ApplicationView) ColorizeStatusText(rowNum int) {
	for i := 0; i < rowNum; i++ {
		status := v.Table.GetCell(i+1, 2).Text
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
		v.Table.GetCell(i+1, 2).SetText(status)
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

func (v *ApplicationView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Enter", Action: v.managedResourceView, Visible: true, Shared: true},
		component.KeyN:    model.KeyAction{Description: "Select Namespace", Action: v.namespaceView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
		component.KeyY:    model.KeyAction{Description: "Yaml", Action: v.yamlView, Visible: true, Shared: true},
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

func (v *ApplicationView) yamlView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text
	gvr := model.GVR{
		GV: "core.oam.dev/v1beta1",
		R: model.Resource{
			Kind:      "Application",
			Name:      name,
			Namespace: namespace,
		},
	}
	ctx := context.WithValue(v.app.ctx, &model.CtxKeyGVR, &gvr)
	v.app.command.run(ctx, "yaml")
	return nil
}
