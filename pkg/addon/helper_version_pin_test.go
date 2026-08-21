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

package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckVersionPinSupported(t *testing.T) {
	cases := map[string]struct {
		requested string
		available string
		wantErr   bool
	}{
		"no pin is always fine":                     {requested: "", available: "1.0.0", wantErr: false},
		"no pin and no version is fine":             {requested: "", available: "", wantErr: false},
		"a pin matching what is served is fine":     {requested: "1.0.0", available: "1.0.0", wantErr: false},
		"a pin the registry cannot honor is an err": {requested: "2.0.0", available: "1.0.0", wantErr: true},
		"a pin against an unversioned addon errors": {requested: "2.0.0", available: "", wantErr: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := checkVersionPinSupported("my-git-registry", "fluxcd", tc.requested, tc.available)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			// The message has to name the registry, the addon and both versions, so
			// the Application status says why the pin was refused.
			assert.Contains(t, err.Error(), "my-git-registry")
			assert.Contains(t, err.Error(), "fluxcd")
			assert.Contains(t, err.Error(), tc.requested)
			assert.Contains(t, err.Error(), "does not support version pinning")
		})
	}
}
