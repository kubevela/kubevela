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
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ApplicationView is the application view, this view display info of application of KubeVela
type ApplicationView struct {
	*CommonResourceView
	ctx context.Context
}

// Name return application view name
func (v *ApplicationView) Name() string {
	return "Application"
}

// Init the application view
func (v *ApplicationView) Init() {
	v.CommonResourceView.Init()
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Title())).SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.bindKeys()
}

// Start the application view
func (v *ApplicationView) Start() {
	v.Clear()
	v.Update(func() {})
	v.CommonResourceView.AutoRefresh(v.Update)
}

// Stop the application view
func (v *ApplicationView) Stop() {
	v.CommonResourceView.Stop()
}

// Hint return key action menu hints of the application view
func (v *ApplicationView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// InitView return a new application view
func (v *ApplicationView) InitView(ctx context.Context, app *App) {
	v.ctx = ctx
	if v.CommonResourceView == nil {
		v.CommonResourceView = NewCommonView(app)
	}
}

// Refresh the view content
func (v *ApplicationView) Refresh(_ *tcell.EventKey) *tcell.EventKey {
	v.CommonResourceView.Refresh(true, v.Update)
	return nil
}

// Update refresh the content of body of view
func (v *ApplicationView) Update(timeoutCancel func()) {
	v.BuildHeader()
	v.BuildBody()
	timeoutCancel()
}

// BuildHeader render the header of table
func (v *ApplicationView) BuildHeader() {
	header := []string{"Name", "Namespace", "Phase", "WorkflowMode", "Workflow", "Service", "CreateTime"}
	v.CommonResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *ApplicationView) BuildBody() {
	apps, err := model.ListApplications(v.ctx, v.app.client)
	if err != nil {
		return
	}
	appInfos := apps.ToTableBody()
	v.CommonResourceView.BuildBody(appInfos)
	rowNum := len(appInfos)
	v.ColorizeStatusText(rowNum)
}

// ColorizeStatusText colorize the status column text
func (v *ApplicationView) ColorizeStatusText(rowNum int) {
	for i := 0; i < rowNum; i++ {
		status := v.Table.GetCell(i+1, 2).Text
		highlightColor := v.app.config.Theme.Table.Body.String()

		switch common.ApplicationPhase(status) {
		case common.ApplicationStarting:
			highlightColor = v.app.config.Theme.Status.Starting.String()
		case common.ApplicationRendering, common.ApplicationPolicyGenerating, common.ApplicationRunningWorkflow, common.ApplicationWorkflowSuspending:
			highlightColor = v.app.config.Theme.Status.Waiting.String()
		case common.ApplicationUnhealthy, common.ApplicationWorkflowTerminated, common.ApplicationWorkflowFailed, common.ApplicationDeleting:
			highlightColor = v.app.config.Theme.Status.Failed.String()
		case common.ApplicationRunning:
			highlightColor = v.app.config.Theme.Status.Healthy.String()
		default:
		}
		v.Table.GetCell(i+1, 2).SetText(fmt.Sprintf("[%s::]%s", highlightColor, status))
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
		tcell.KeyESC:   model.KeyAction{Description: "Exist", Action: v.app.Exist, Visible: true, Shared: true},
		tcell.KeyEnter: model.KeyAction{Description: "Managed Resource", Action: v.managedResourceView, Visible: true, Shared: true},
		component.KeyN: model.KeyAction{Description: "Select Namespace", Action: v.namespaceView, Visible: true, Shared: true},
		component.KeyY: model.KeyAction{Description: "Yaml", Action: v.yamlView, Visible: true, Shared: true},
		component.KeyR: model.KeyAction{Description: "Refresh", Action: v.Refresh, Visible: true, Shared: true},
		component.KeyT: model.KeyAction{Description: "Topology", Action: v.topologyView, Visible: true, Shared: true},
	})
}

func (v *ApplicationView) managedResourceView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}

	name, namespace := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text
	ctx := context.WithValue(v.ctx, &model.CtxKeyAppName, name)
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, namespace)
	v.app.command.run(ctx, "resource")
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

func (v *ApplicationView) topologyView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text

	ctx := context.WithValue(context.Background(), &model.CtxKeyAppName, name)
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, namespace)

	v.app.command.run(ctx, "topology")
	return nil
}
