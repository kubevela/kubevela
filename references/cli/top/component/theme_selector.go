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
	"fmt"
	"sort"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/config"
)

// ThemeSelector is used to select the theme
type ThemeSelector struct {
	Frame     *tview.Frame
	list      *tview.List
	style     *config.ThemeConfig
	closeFunc func()
}

// NewThemeSelector is used to create a new theme selector
func NewThemeSelector(config *config.ThemeConfig, closeFun func()) *ThemeSelector {
	s := &ThemeSelector{
		style:     config,
		closeFunc: closeFun,
	}
	s.list = tview.NewList()
	s.Frame = tview.NewFrame(s.list)
	return s
}

// Init is used to initialize the theme selector
func (s *ThemeSelector) Init() {
	s.Frame.SetBorder(true)
	s.Frame.SetBorderColor(s.style.Border.Table.Color())
	s.Frame.SetBorders(2, 2, 2, 2, 4, 4)
	s.Frame.AddText(themeSelectTip(), true, tview.AlignCenter, s.style.Info.Title.Color())
	s.Frame.SetTitle(fmt.Sprintf("[ %s ]", "Select Theme"))
	s.Frame.SetTitleColor(s.style.Table.Title.Color())

	s.list.SetShortcutColor(s.style.Info.Title.Color())
	s.list.SetMainTextColor(s.style.Info.Text.Color())
}

// Start is used to start the theme selector
func (s *ThemeSelector) Start() {
	sort.Strings(config.ThemeNameArray)
	for _, key := range config.ThemeNameArray {
		s.list.AddItem(key, "", '*', s.selectedFunc(key))
	}
}

func (s *ThemeSelector) selectedFunc(theme string) func() {
	return func() {
		config.PersistentThemeConfig(theme)
		s.closeFunc()
	}
}

func themeSelectTip() string {
	return "Restart vela top to take effect"
}
