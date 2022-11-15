/*
Copyright 2021 The KubeVela Authors.

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

package cli

import (
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	"github.com/kyokomi/emoji"
)

// colors used in vela cmd for printing
var (
	red    = color.New(color.FgRed)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	white  = color.New(color.Bold, color.FgWhite)
)

// emoji used in vela cmd for printing
var (
	emojiSucceed   = emoji.Sprint(":check_mark_button:")
	emojiFail      = emoji.Sprint(":cross_mark:")
	emojiExecuting = emoji.Sprint(":hourglass:")
	emojiSkip      = emoji.Sprint(":no_entry:")
)

// newUITable creates a new table with fixed MaxColWidth
func newUITable() *uitable.Table {
	t := uitable.New()
	t.MaxColWidth = 60
	t.Wrap = true
	return t
}

func newTrackingSpinnerWithDelay(suffix string, interval time.Duration) *spinner.Spinner {
	suffixColor := color.New(color.Bold, color.FgWhite)
	return spinner.New(
		spinner.CharSets[14],
		interval,
		spinner.WithColor("white"),
		spinner.WithHiddenCursor(true),
		spinner.WithSuffix(suffixColor.Sprintf(" %s", suffix)))
}

func newTrackingSpinner(suffix string) *spinner.Spinner {
	return newTrackingSpinnerWithDelay(suffix, 500*time.Millisecond)
}

func applySpinnerNewSuffix(s *spinner.Spinner, suffix string) {
	suffixColor := color.New(color.Bold, color.FgGreen)
	s.Suffix = suffixColor.Sprintf(" %s", suffix)
}
