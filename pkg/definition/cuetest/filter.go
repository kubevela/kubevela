/*
Copyright 2026 The KubeVela Authors.

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

package cuetest

import (
	"errors"
	"fmt"
	"regexp"

	ginkgotypes "github.com/onsi/ginkgo/v2/types"
)

// FilterOptions selects cases the way Ginkgo's flags of the same names select
// specs, so a filter picks the same cases in `vela def test` and in a Ginkgo
// suite built with ginkgotest.
type FilterOptions struct {
	// LabelFilter is a Ginkgo label query, such as "status && !slow".
	LabelFilter string
	// Focus keeps only cases whose full text matches any of these regexps.
	Focus []string
	// Skip drops cases whose full text matches any of these regexps.
	Skip []string
	// FocusFiles keeps only cases in files matching any of these regexps,
	// each optionally followed by lines: file:12, file:3,9 or file:1-20
	// (half-open, as in Ginkgo).
	FocusFiles []string
}

// Filter decides which cases run.
type Filter struct {
	labels      ginkgotypes.LabelFilter
	focus, skip []*regexp.Regexp
	files       ginkgotypes.FileFilters
}

// NewFilter compiles opts.
func NewFilter(opts FilterOptions) (*Filter, error) {
	f := &Filter{}
	var err error
	if f.labels, err = ginkgotypes.ParseLabelFilter(opts.LabelFilter); err != nil {
		return nil, fmt.Errorf("--label-filter: %w", ginkgoError(err))
	}
	if f.focus, err = compileAll("--focus", opts.Focus); err != nil {
		return nil, err
	}
	if f.skip, err = compileAll("--skip", opts.Skip); err != nil {
		return nil, err
	}
	if f.files, err = ginkgotypes.ParseFileFilters(opts.FocusFiles); err != nil {
		return nil, fmt.Errorf("--focus-file: %w", ginkgoError(err))
	}
	return f, nil
}

// Match reports whether c passes every filter.
func (f *Filter) Match(c *Case) bool {
	text := c.FullText()
	if len(f.focus) > 0 && !anyMatch(f.focus, text) {
		return false
	}
	if anyMatch(f.skip, text) {
		return false
	}
	if len(f.files) > 0 && !f.files.Matches([]ginkgotypes.CodeLocation{{FileName: c.File, LineNumber: c.Line}}) {
		return false
	}
	return f.labels(c.Labels)
}

// FullText is what --focus and --skip match: the path the case was loaded
// from, its test file, then the definition and case name, as Ginkgo joins a
// ginkgotest spec's containers and name.
func (c *Case) FullText() string {
	return fmt.Sprintf("%s %s %s / %s", c.Path, c.File, c.Subject.Name, c.Name)
}

func compileAll(flag string, patterns []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", flag, p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func anyMatch(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// ginkgoError drops the terminal formatting from Ginkgo's parse errors.
func ginkgoError(err error) error {
	var gerr ginkgotypes.GinkgoError
	if errors.As(err, &gerr) {
		return errors.New(gerr.Message)
	}
	return err
}
