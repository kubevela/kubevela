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

	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// InfoBoard a component which display system info
type InfoBoard struct {
	*tview.Table
}

// NewInfo return an info component instance
func NewInfo() *InfoBoard {
	c := &InfoBoard{
		Table: tview.NewTable(),
	}
	return c
}

// Init info component init
func (board *InfoBoard) Init(restConf *rest.Config) {
	board.layout()
	board.UpdateInfo(restConf)
}

func (board *InfoBoard) layout() {
	row := 0
	board.SetCell(row, 0, board.sectionCell("Context").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 0, board.sectionCell("K8S Version").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 0, board.sectionCell("VelaCLI Version").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 0, board.sectionCell("VelaCore Version").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 0, board.sectionCell("Cluster Num").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 0, board.sectionCell("App Running Num").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 2, infoCell("|").SetTextColor(config.InfoSectionColor))

	row = 0
	board.SetCell(row, 3, infoCell("").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 4, infoCell("VelaCore").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 5, infoCell("ClusterGateway").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 3, infoCell("").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 4, infoCell("").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 5, infoCell("").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 3, board.sectionCell("CPU Requests").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 3, board.sectionCell("CPU Limits").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 3, board.sectionCell("MEM Requests").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(config.InfoSectionColor))
	row++

	board.SetCell(row, 3, board.sectionCell("MEM Limits").SetTextColor(config.InfoSectionColor))
	board.SetCell(row, 6, infoCell("|").SetTextColor(config.InfoSectionColor))
}

// UpdateInfo update the info of system info board
func (board *InfoBoard) UpdateInfo(restConf *rest.Config) {
	row := 0
	info := model.NewInfo()
	board.SetCell(row, 1, infoCell(info.CurrentContext()).SetTextColor(config.InfoTextColor))
	row++

	k8s := model.K8SVersion(restConf)
	board.SetCell(row, 1, infoCell(k8s).SetTextColor(config.InfoTextColor))
	row++

	velaCLI := model.VelaCLIVersion()
	board.SetCell(row, 1, infoCell(velaCLI).SetTextColor(config.InfoTextColor))
	row++

	velaCore := model.VelaCoreVersion()
	board.SetCell(row, 1, infoCell(velaCore).SetTextColor(config.InfoTextColor))
	row++

	clusterNum := info.ClusterNum()
	board.SetCell(row, 1, infoCell(clusterNum).SetTextColor(config.InfoTextColor))
	row++

	appNum := model.ApplicationRunningNum(restConf)
	board.SetCell(row, 1, infoCell(appNum).SetTextColor(config.InfoTextColor))

	velaCoreCPULimit, velaCoreMEMLimit, velaCoreCPURequest, velaCoreMEMRequest := model.VelaCoreRatio(restConf)
	gatewayCPULimit, gatewayMEMLimit, gatewayCPURequest, gatewayMEMRequest := model.CLusterGatewayRatio(restConf)

	row = 2

	board.SetCell(row, 4, infoCell(velaCoreCPURequest).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayCPURequest).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
	row++

	board.SetCell(row, 4, infoCell(velaCoreCPULimit).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayCPULimit).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
	row++

	board.SetCell(row, 4, infoCell(velaCoreMEMRequest).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayMEMRequest).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
	row++

	board.SetCell(row, 4, infoCell(velaCoreMEMLimit).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
	board.SetCell(row, 5, infoCell(gatewayMEMLimit).SetTextColor(config.InfoTextColor).SetAlign(tview.AlignCenter))
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
