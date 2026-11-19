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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilter(t *testing.T) {
	cases := []*Case{
		{Name: "defaults to one replica", Path: "defs", File: "defs/web_test.cue", Line: 3, Subject: Subject{Name: "webservice"}, Labels: []string{"component", "render"}},
		{Name: "healthy once ready", Path: "defs", File: "defs/web_test.cue", Line: 9, Subject: Subject{Name: "webservice"}, Labels: []string{"component", "slow", "status"}},
		{Name: "patches replicas", Path: "defs", File: "defs/scaler_test.cue", Line: 3, Subject: Subject{Name: "scaler"}, Labels: []string{"render", "trait"}},
	}
	tests := map[string]struct {
		opts FilterOptions
		want []string
	}{
		"no filter":            {FilterOptions{}, []string{"defaults to one replica", "healthy once ready", "patches replicas"}},
		"label":                {FilterOptions{LabelFilter: "status"}, []string{"healthy once ready"}},
		"label expression":     {FilterOptions{LabelFilter: "component && !slow"}, []string{"defaults to one replica"}},
		"label regex":          {FilterOptions{LabelFilter: "/^tr/"}, []string{"patches replicas"}},
		"focus on case name":   {FilterOptions{Focus: []string{"replica"}}, []string{"defaults to one replica", "patches replicas"}},
		"focus on definition":  {FilterOptions{Focus: []string{"^\\S+ \\S+ scaler /"}}, []string{"patches replicas"}},
		"focus is any-of":      {FilterOptions{Focus: []string{"healthy", "patches"}}, []string{"healthy once ready", "patches replicas"}},
		"skip":                 {FilterOptions{Skip: []string{"replica"}}, []string{"healthy once ready"}},
		"focus then skip":      {FilterOptions{Focus: []string{"webservice"}, Skip: []string{"healthy"}}, []string{"defaults to one replica"}},
		"focus file":           {FilterOptions{FocusFiles: []string{"scaler_test"}}, []string{"patches replicas"}},
		"focus file and line":  {FilterOptions{FocusFiles: []string{"web_test.cue:9"}}, []string{"healthy once ready"}},
		"focus file and range": {FilterOptions{FocusFiles: []string{"web_test.cue:1-9"}}, []string{"defaults to one replica"}},
		"focus file line list": {FilterOptions{FocusFiles: []string{"web_test.cue:3,9"}}, []string{"defaults to one replica", "healthy once ready"}},
		"all together":         {FilterOptions{LabelFilter: "component", Focus: []string{"once"}, FocusFiles: []string{"web_test"}}, []string{"healthy once ready"}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := NewFilter(tc.opts)
			require.NoError(t, err)
			var got []string
			for _, c := range cases {
				if f.Match(c) {
					got = append(got, c.Name)
				}
			}
			require.Equal(t, tc.want, got)
		})
	}
}

func TestFilterErrors(t *testing.T) {
	for opts, want := range map[*FilterOptions]string{
		{LabelFilter: "a &&"}:             "--label-filter",
		{Focus: []string{"("}}:            "--focus",
		{Skip: []string{"("}}:             "--skip",
		{FocusFiles: []string{"("}}:       "--focus-file",
		{FocusFiles: []string{"a.cue:x"}}: "--focus-file",
	} {
		_, err := NewFilter(*opts)
		require.ErrorContains(t, err, want)
	}
}

func TestFullText(t *testing.T) {
	c := &Case{Name: "healthy once ready", Path: "defs", File: "defs/web_test.cue", Subject: Subject{Name: "webservice"}}
	require.Equal(t, "defs defs/web_test.cue webservice / healthy once ready", c.FullText())
}
