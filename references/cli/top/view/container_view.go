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

// ContainerView is a view which displays info of container of aime pod
type ContainerView struct {
	*CommonResourceView
	ctx context.Context
}

// Init container view
func (v *ContainerView) Init() {
	v.CommonResourceView.Init()
	// set title of view
	v.SetTitle(fmt.Sprintf("[ %s ]", "Container")).SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.bindKeys()
}

// Name return pod container name
func (v *ContainerView) Name() string {
	return "Container"
}

// Start the container view
func (v *ContainerView) Start() {
	v.Clear()
	v.Update(func() {})
	v.CommonResourceView.AutoRefresh(v.Update)
}

// Stop the container view
func (v *ContainerView) Stop() {
	v.CommonResourceView.Stop()
}

// Hint return key action menu hints of the container view
func (v *ContainerView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// InitView init a new container view
func (v *ContainerView) InitView(ctx context.Context, app *App) {
	v.ctx = ctx
	if v.CommonResourceView == nil {
		v.CommonResourceView = NewCommonView(app)
	}
}

// Refresh the view content
func (v *ContainerView) Refresh(_ *tcell.EventKey) *tcell.EventKey {
	v.CommonResourceView.Refresh(true, v.Update)
	return nil
}

// Update refresh the content of body of view
func (v *ContainerView) Update(timeoutCancel func()) {
	v.BuildHeader()
	v.BuildBody()
	timeoutCancel()
}

// BuildHeader render the header of table
func (v *ContainerView) BuildHeader() {
	header := []string{"Name", "Image", "Ready", "State", "CPU", "MEM", "CPU/R", "CPU/L", "MEM/R", "MEM/L", "TerminateMessage", "RestartCount"}
	v.CommonResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *ContainerView) BuildBody() {
	containerList, err := model.ListContainerOfPod(v.ctx, v.app.client, v.app.config.RestConfig)
	if err != nil {
		return
	}
	resourceInfos := containerList.ToTableBody()
	v.CommonResourceView.BuildBody(resourceInfos)

	rowNum := len(containerList)
	v.ColorizePhaseText(rowNum)
}

func (v *ContainerView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		component.KeyL: model.KeyAction{Description: "Log", Action: v.logView, Visible: true, Shared: true},
	})
}

// ColorizePhaseText colorize the state column text
func (v *ContainerView) ColorizePhaseText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		state := v.Table.GetCell(i, 3).Text
		highlightColor := v.app.config.Theme.Table.Body.String()

		switch common.ContainerState(state) {
		case common.ContainerRunning:
			highlightColor = v.app.config.Theme.Status.Healthy.String()
		case common.ContainerWaiting:
			highlightColor = v.app.config.Theme.Status.Waiting.String()
		case common.ContainerTerminated:
			highlightColor = v.app.config.Theme.Status.UnHealthy.String()
		default:
		}
		v.Table.GetCell(i, 3).SetText(fmt.Sprintf("[%s::]%s", highlightColor, state))
	}
}

func (v *ContainerView) logView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}

	name := v.GetCell(row, 0).Text
	ctx := context.WithValue(v.ctx, &model.CtxKeyContainer, name)

	v.app.command.run(ctx, "log")
	return nil
}
