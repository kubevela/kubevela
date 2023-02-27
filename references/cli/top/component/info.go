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

package component

import (
	"github.com/rivo/tview"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// InfoBoard a component which display system info
type InfoBoard struct {
	*tview.Table
	style *config.ThemeConfig
}

// NewInfo return an info component instance
func NewInfo(config *config.ThemeConfig) *InfoBoard {
	c := &InfoBoard{
		Table: tview.NewTable(),
		style: config,
	}
	return c
}

// Init info component init
func (board *InfoBoard) Init(c client.Client, restConf *rest.Config) {
	board.layout()
	board.UpdateInfo(c, restConf)
}

func (board *InfoBoard) layout() {
	titleColor := board.style.Info.Title.Color()
	row := 0
	board.SetCell(row, 0, board.sectionCell("Context").SetTextColor(titleColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 0, board.sectionCell("K8S Version").SetTextColor(titleColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 0, board.sectionCell("VelaCLI Version").SetTextColor(titleColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 0, board.sectionCell("VelaCore Version").SetTextColor(titleColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 0, board.sectionCell("Cluster Num").SetTextColor(titleColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 0, board.sectionCell("Running App Num").SetTextColor(titleColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(titleColor))

	row = 0
	board.SetCell(row, 3, infoCell("").SetTextColor(titleColor))
	board.SetCell(row, 4, infoCell("VelaCore").SetTextColor(titleColor))
	board.SetCell(row, 5, infoCell("Gateway").SetTextColor(titleColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 3, infoCell("").SetTextColor(titleColor))
	board.SetCell(row, 4, infoCell("").SetTextColor(titleColor))
	board.SetCell(row, 5, infoCell("").SetTextColor(titleColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 3, board.sectionCell("CPU Requests").SetTextColor(titleColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 3, board.sectionCell("CPU Limits").SetTextColor(titleColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 3, board.sectionCell("MEM Requests").SetTextColor(titleColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(titleColor))
	row++

	board.SetCell(row, 3, board.sectionCell("MEM Limits").SetTextColor(titleColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(titleColor))
}

// UpdateInfo update the info of system info board
func (board *InfoBoard) UpdateInfo(c client.Client, restConf *rest.Config) {
	textColor := board.style.Info.Text.Color()
	row := 0
	info := model.NewInfo()
	board.SetCell(row, 1, infoCell(info.CurrentContext()).SetTextColor(textColor))
	row++

	k8s := model.K8SVersion(restConf)
	board.SetCell(row, 1, infoCell(k8s).SetTextColor(textColor))
	row++

	velaCLI := model.VelaCLIVersion()
	board.SetCell(row, 1, infoCell(velaCLI).SetTextColor(textColor))
	row++

	velaCore := model.VelaCoreVersion()
	board.SetCell(row, 1, infoCell(velaCore).SetTextColor(textColor))
	row++

	clusterNum := info.ClusterNum()
	board.SetCell(row, 1, infoCell(clusterNum).SetTextColor(textColor))
	row++

	appNum := model.ApplicationRunningNum(restConf)
	board.SetCell(row, 1, infoCell(appNum).SetTextColor(textColor))

	velaCoreCPULimit, velaCoreMEMLimit, velaCoreCPURequest, velaCoreMEMRequest := model.VelaCoreRatio(c, restConf)
	gatewayCPULimit, gatewayMEMLimit, gatewayCPURequest, gatewayMEMRequest := model.CLusterGatewayRatio(c, restConf)

	row = 2

	board.SetCell(row, 4, infoCell(velaCoreCPURequest).SetTextColor(textColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayCPURequest).SetTextColor(textColor).SetAlign(tview.AlignCenter))
	row++

	board.SetCell(row, 4, infoCell(velaCoreCPULimit).SetTextColor(textColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayCPULimit).SetTextColor(textColor).SetAlign(tview.AlignCenter))
	row++

	board.SetCell(row, 4, infoCell(velaCoreMEMRequest).SetTextColor(textColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayMEMRequest).SetTextColor(textColor).SetAlign(tview.AlignCenter))
	row++

	board.SetCell(row, 4, infoCell(velaCoreMEMLimit).SetTextColor(textColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayMEMLimit).SetTextColor(textColor).SetAlign(tview.AlignCenter))
}

func (board *InfoBoard) sectionCell(t string) *tview.TableCell {
	c := tview.NewTableCell(t + ":")
	c.SetAlign(tview.AlignLeft)
	return c
}

func infoCell(t string) *tview.TableCell {
	c := tview.NewTableCell(t)
	return c
}
