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

package dryrun

import (
	"fmt"
	"io"
	"math"

	"github.com/aryann/difflib"
	"github.com/fatih/color"
)

var (
	red    = color.New(color.FgRed)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	white  = color.New(color.FgWhite)
)

// NewReportDiffOption creats a new ReportDiffOption that can formats and prints
// diff report into an io.Writer
func NewReportDiffOption(ctx int, to io.Writer) *ReportDiffOption {
	return &ReportDiffOption{
		DiffMsgs: map[DiffType]string{
			AddDiff:    "has been added(+)",
			ModifyDiff: "has been modified(*)",
			RemoveDiff: "has been removed(-)",
			NoDiff:     "has no change",
		},
		Context: ctx,
		To:      to,
	}
}

// ReportDiffOption contains options to formats and prints diff report
type ReportDiffOption struct {
	DiffMsgs map[DiffType]string
	Context  int
	To       io.Writer
}

// PrintDiffReport formats and prints diff data into target io.Writer
func (r *ReportDiffOption) PrintDiffReport(diff *DiffEntry) {
	r.printDiffReport(diff, "")
}

func (r *ReportDiffOption) printDiffReport(diff *DiffEntry, prefix string) {
	var header string
	switch diff.Kind {
	case AppKind:
		header = "Application"
	case AppConfigCompKind:
	case RawCompKind:
		header = "Component"
	case TraitKind:
		header = "Trait"
	case PolicyKind:
		header = "External Policy"
	case WorkflowKind:
		header = "External Workflow"
	case ReferredObject:
		header = "Referred Object"
	default:
		return
	}
	if diff.Kind != AppConfigCompKind {
		editMsg := r.DiffMsgs[diff.DiffType]
		if diff.DiffType != NoDiff {
			_, _ = yellow.Fprintf(r.To, "* %s%s (%s) %s\n", prefix, header, diff.Name, editMsg)
			printDiffs(diff.Diffs, r.Context, r.To)
		} else {
			_, _ = white.Fprintf(r.To, "* %s%s (%s) %s\n", prefix, header, diff.Name, editMsg)
		}
	}
	for _, sub := range diff.Subs {
		var subPrefix string
		if sub.Kind == TraitKind && diff.Kind == AppConfigCompKind {
			subPrefix = fmt.Sprintf("Component (%s) / ", diff.Name)
		}
		r.printDiffReport(sub, subPrefix)
	}
}

func printDiffs(diffs []difflib.DiffRecord, context int, to io.Writer) {
	if context > 0 {
		ctx := calculateContext(diffs)
		skip := false
		for i, diff := range diffs {
			if ctx[i] <= context {
				// only print the line whose distance to a closest diff is less
				// than context
				printDiffRecord(to, diff)
				skip = false
			} else if !skip {
				fmt.Fprint(to, "...\n")
				// skip print if next line is still omitted
				skip = true
			}

		}
	} else {
		for _, diff := range diffs {
			printDiffRecord(to, diff)
		}
	}
}

// calculateContext calculate the min distance from each line to its closest diff
func calculateContext(diffs []difflib.DiffRecord) map[int]int {
	ctx := map[int]int{}
	// retrieve forward to calculate the min distance from each line to a
	// changed line behind it
	changeLineNum := -1
	for i, diff := range diffs {
		if diff.Delta != difflib.Common {
			changeLineNum = i
		}
		distance := math.MaxInt32
		if changeLineNum != -1 {
			distance = i - changeLineNum
		}
		ctx[i] = distance
	}
	// retrieve backward to calculate the min distance from each line to a
	// changed line before it
	changeLineNum = -1
	for i := len(diffs) - 1; i >= 0; i-- {
		if diffs[i].Delta != difflib.Common {
			changeLineNum = i
		}
		if changeLineNum != -1 {
			distance := changeLineNum - i
			if distance < ctx[i] {
				ctx[i] = distance
			}
		}
	}
	return ctx
}

func printDiffRecord(to io.Writer, diff difflib.DiffRecord) {
	data := diff.Payload
	switch diff.Delta {
	case difflib.RightOnly:
		_, _ = green.Fprintf(to, "+ %s\n", data)
	case difflib.LeftOnly:
		_, _ = red.Fprintf(to, "- %s\n", data)
	case difflib.Common:
		_, _ = fmt.Fprintf(to, "  %s\n", data)
	}
}
