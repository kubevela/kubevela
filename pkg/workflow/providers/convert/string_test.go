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

package convert

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

func TestConvertString(t *testing.T) {
	testCases := map[string]struct {
		from        string
		expected    string
		expectedErr error
	}{
		"success": {
			from:     `bt: 'test'`,
			expected: "test",
		},
		"fail": {
			from:        `bt: 123`,
			expectedErr: errors.New("bt: cannot use value 123 (type int) as string|bytes"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			v, err := value.NewValue(tc.from, nil, "")
			r.NoError(err)
			prd := &provider{}
			err = prd.String(nil, v, nil)
			if tc.expectedErr != nil {
				r.Equal(tc.expectedErr.Error(), err.Error())
				return
			}
			r.NoError(err)
			expected, err := v.LookupValue("str")
			r.NoError(err)
			ret, err := expected.CueValue().String()
			r.NoError(err)
			r.Equal(ret, tc.expected)
		})
	}
}

func TestInstall(t *testing.T) {
	p := providers.NewProviders()
	Install(p)
	h, ok := p.GetHandler("convert", "string")
	r := require.New(t)
	r.Equal(ok, true)
	r.Equal(h != nil, true)
}
