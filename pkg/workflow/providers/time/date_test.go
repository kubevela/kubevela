/*
 Copyright 2021. The KubeVela Authors.

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

package time

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/providers"
)

func TestTimestamp(t *testing.T) {
	testcases := map[string]struct {
		from        string
		expected    int64
		expectedErr error
	}{
		"test convert date with default time layout": {
			from: `date: "2021-11-07T01:47:51Z"
layout: ""`,
			expected:    1636249671,
			expectedErr: nil,
		},
		"test convert date with RFC3339 layout": {
			from: `date: "2021-11-07T01:47:51Z"
layout: "2006-01-02T15:04:05Z07:00"`,
			expected:    1636249671,
			expectedErr: nil,
		},
		"test convert date with RFC1123 layout": {
			from: `date: "Fri, 01 Mar 2019 15:00:00 GMT"
layout: "Mon, 02 Jan 2006 15:04:05 MST"`,
			expected:    1551452400,
			expectedErr: nil,
		},
		"test convert date without time layout": {
			from:        `date: "2021-11-07T01:47:51Z"`,
			expected:    0,
			expectedErr: errors.New("failed to lookup value: var(path=layout) not exist"),
		},
		"test convert without date": {
			from:        ``,
			expected:    0,
			expectedErr: errors.New("failed to lookup value: var(path=date) not exist"),
		},
		"test convert date with wrong time layout": {
			from: `date: "2021-11-07T01:47:51Z"
layout: "Mon, 02 Jan 2006 15:04:05 MST"`,
			expected:    0,
			expectedErr: errors.New(`parsing time "2021-11-07T01:47:51Z" as "Mon, 02 Jan 2006 15:04:05 MST": cannot parse "2021-11-07T01:47:51Z" as "Mon"`),
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			v, err := value.NewValue(tc.from, nil, "")
			r.NoError(err)
			prd := &provider{}
			err = prd.Timestamp(nil, nil, v, nil)
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr.Error(), err.Error())
				return
			}
			r.NoError(err)
			expected, err := v.LookupValue("timestamp")
			r.NoError(err)
			ret, err := expected.CueValue().Int64()
			r.NoError(err)
			r.Equal(tc.expected, ret)
		})
	}
}

func TestDate(t *testing.T) {
	testcases := map[string]struct {
		from        string
		expected    string
		expectedErr error
	}{
		"test convert timestamp to default time layout": {
			from: `timestamp: 1636249671
layout: ""
`,
			expected:    "2021-11-07T01:47:51Z",
			expectedErr: nil,
		},
		"test convert date to RFC3339 layout": {
			from: `timestamp: 1636249671
layout: "2006-01-02T15:04:05Z07:00"
`,
			expected:    "2021-11-07T01:47:51Z",
			expectedErr: nil,
		},
		"test convert date to RFC1123 layout": {
			from: `timestamp: 1551452400
layout: "Mon, 02 Jan 2006 15:04:05 MST"
`,
			expected:    "Fri, 01 Mar 2019 15:00:00 UTC",
			expectedErr: nil,
		},
		"test convert date without time layout": {
			from:        `timestamp: 1551452400`,
			expected:    "",
			expectedErr: errors.New("failed to lookup value: var(path=layout) not exist"),
		},
		"test convert without timestamp": {
			from:        ``,
			expected:    "",
			expectedErr: errors.New("failed to lookup value: var(path=timestamp) not exist"),
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			v, err := value.NewValue(tc.from, nil, "")
			r.NoError(err)
			prd := &provider{}
			err = prd.Date(nil, nil, v, nil)
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr.Error(), err.Error())
				return
			}
			r.NoError(err)
			expected, err := v.LookupValue("date")
			r.NoError(err)
			ret, err := expected.CueValue().String()
			r.NoError(err)
			r.Equal(tc.expected, ret)
		})
	}
}

func TestInstall(t *testing.T) {
	p := providers.NewProviders()
	Install(p)
	h, ok := p.GetHandler("time", "timestamp")
	r := require.New(t)
	r.Equal(ok, true)
	r.Equal(h != nil, true)

	h, ok = p.GetHandler("time", "date")
	r.Equal(ok, true)
	r.Equal(h != nil, true)
}
