package commands

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
	emojiLightBulb = emoji.Sprint(":light_bulb:")
)

// newUITable creates a new table with fixed MaxColWidth
func newUITable() *uitable.Table {
	t := uitable.New()
	t.MaxColWidth = 60
	t.Wrap = true
	return t
}

func newTrackingSpinnerWithDelay(suffix string, interval time.Duration) *spinner.Spinner {
	suffixColor := color.New(color.Bold, color.FgGreen)
	return spinner.New(
		spinner.CharSets[14],
		interval,
		spinner.WithColor("green"),
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
