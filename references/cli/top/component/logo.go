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

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/config"
)

var velaLogo = []string{
	` _  __       _          __     __     _	    `,
	`| |/ /_   _ | |__    ___\ \   / /___ | |  __ _ `,
	`| ' /| | | || '_ \  / _ \\ \ / // _ \| | / _\ |`,
	`| . \| |_| || |_) ||  __/ \ V /|  __/| || (_| |`,
	`|_|\_\\__,_||_.__/  \___|  \_/  \___||_| \__,_|`,
}

// Logo logo component in header
type Logo struct {
	*tview.TextView
	style *config.ThemeConfig
}

// NewLogo return logo ui component
func NewLogo(config *config.ThemeConfig) *Logo {
	l := &Logo{
		TextView: tview.NewTextView(),
		style:    config,
	}
	l.init()
	return l
}

func (l *Logo) init() {
	l.SetWrap(false)
	l.SetWordWrap(false)
	l.SetTextAlign(tview.AlignCenter)
	l.SetTextColor(l.style.Logo.Text.Color())
	l.SetDynamicColors(true)
	for _, line := range velaLogo {
		fmt.Fprintf(l, "%s\n", line)
	}
}
