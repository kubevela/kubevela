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
	v1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// PodView is the pod view, this view display info of pod belonging to component
type PodView struct {
	*CommonResourceView
	ctx context.Context
}

// Name return pod view name
func (v *PodView) Name() string {
	return "Pod"
}

// Start the pod view
func (v *PodView) Start() {
	v.Clear()
	v.Update(func() {})
	v.AutoRefresh(v.Update)
}

// Stop the pod view
func (v *PodView) Stop() {
	v.CommonResourceView.Stop()
}

// Hint return key action menu hints of the pod view
func (v *PodView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// Init cluster view init
func (v *PodView) Init() {
	v.CommonResourceView.Init()
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Name()))
	v.bindKeys()
}

// InitView init a new pod view
func (v *PodView) InitView(ctx context.Context, app *App) {
	v.ctx = ctx
	if v.CommonResourceView == nil {
		v.CommonResourceView = NewCommonView(app)
	}
}

// Refresh the view content
func (v *PodView) Refresh(_ *tcell.EventKey) *tcell.EventKey {
	v.CommonResourceView.Refresh(true, v.Update)
	return nil
}

// Update refresh the content of body of view
func (v *PodView) Update(timeoutCancel func()) {
	v.BuildHeader()
	v.BuildBody()
	timeoutCancel()
}

// BuildHeader render the header of table
func (v *PodView) BuildHeader() {
	header := []string{"Name", "Namespace", "Cluster", "Ready", "Status", "CPU", "MEM", "CPU/R", "CPU/L", "MEM/R", "MEM/L", "IP", "Node", "Age"}
	v.CommonResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *PodView) BuildBody() {
	podList, err := model.ListPods(v.ctx, v.app.config.RestConfig, v.app.client)
	if err != nil {
		return
	}
	podInfos := podList.ToTableBody()
	v.CommonResourceView.BuildBody(podInfos)
	rowNum := len(podInfos)
	v.ColorizePhaseText(rowNum)
}

// ColorizePhaseText colorize the phase column text
func (v *PodView) ColorizePhaseText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		phase := v.Table.GetCell(i, 4).Text
		highlightColor := v.app.config.Theme.Table.Body.String()

		switch v1.PodPhase(phase) {
		case v1.PodPending:
			highlightColor = v.app.config.Theme.Status.Waiting.String()
		case v1.PodRunning:
			highlightColor = v.app.config.Theme.Status.Healthy.String()
		case v1.PodSucceeded:
			highlightColor = v.app.config.Theme.Status.Succeeded.String()
		case v1.PodFailed:
			highlightColor = v.app.config.Theme.Status.UnHealthy.String()
		case v1.PodUnknown:
			highlightColor = v.app.config.Theme.Status.Unknown.String()
		default:
		}
		v.Table.GetCell(i, 4).SetText(fmt.Sprintf("[%s::]%s", highlightColor, phase))
	}
}

func (v *PodView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter: model.KeyAction{Description: "Containers", Action: v.containerView, Visible: true, Shared: true},
		component.KeyY: model.KeyAction{Description: "Yaml", Action: v.yamlView, Visible: true, Shared: true},
		component.KeyR: model.KeyAction{Description: "Refresh", Action: v.Refresh, Visible: true, Shared: true},
		component.KeyL: model.KeyAction{Description: "Log", Action: v.logView, Visible: true, Shared: true},
	})
}

func (v *PodView) yamlView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text

	gvr := model.GVR{
		GV: "v1",
		R: model.Resource{
			Kind:      "Pod",
			Name:      name,
			Namespace: namespace,
		},
	}
	ctx := context.WithValue(v.app.ctx, &model.CtxKeyGVR, &gvr)
	v.app.command.run(ctx, "yaml")
	return nil
}

func (v *PodView) containerView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace, cluster := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text, v.GetCell(row, 2).Text

	ctx := context.WithValue(context.Background(), &model.CtxKeyPod, name)
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, namespace)
	ctx = context.WithValue(ctx, &model.CtxKeyCluster, cluster)

	v.app.command.run(ctx, "container")
	return nil
}

func (v *PodView) logView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace, cluster := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text, v.GetCell(row, 2).Text

	ctx := context.Background()
	ctx = context.WithValue(ctx, &model.CtxKeyPod, name)
	ctx = context.WithValue(ctx, &model.CtxKeyNamespace, namespace)
	ctx = context.WithValue(ctx, &model.CtxKeyCluster, cluster)

	v.app.command.run(ctx, "log")
	return nil
}
