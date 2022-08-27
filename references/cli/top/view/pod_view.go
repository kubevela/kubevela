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
	v1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// PodView is the pod view, this view display info of pod belonging to component
type PodView struct {
	*ResourceView
	ctx context.Context
}

// NewPodView return a new pod view
func NewPodView(ctx context.Context, app *App) model.Component {
	v := &PodView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init cluster view init
func (v *PodView) Init() {
	// set title of view
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)

	resourceList := v.ListPods()
	v.ResourceView.Init(resourceList)
	v.ColorizePhaseText(len(resourceList.Body()))

	v.bindKeys()
}

// ListPods list pods of component
func (v *PodView) ListPods() model.ResourceList {
	list, err := model.ListPods(v.ctx, v.app.config.RestConfig, v.app.client)
	if err != nil {
		log.Println(err)
	}
	return list
}

// ColorizePhaseText colorize the phase column text
func (v *PodView) ColorizePhaseText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		phase := v.Table.GetCell(i, 3).Text
		switch v1.PodPhase(phase) {
		case v1.PodPending:
			phase = config.PodPendingPhaseColor + phase
		case v1.PodRunning:
			phase = config.PodRunningPhaseColor + phase
		case v1.PodSucceeded:
			phase = config.PodSucceededPhase + phase
		case v1.PodFailed:
			phase = config.PodFailedPhase + phase
		default:
		}
		v.Table.GetCell(i, 3).SetText(phase)
	}
}

// Name return pod view name
func (v *PodView) Name() string {
	return "Pod"
}

// Hint return key action menu hints of the pod view
func (v *PodView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *PodView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
