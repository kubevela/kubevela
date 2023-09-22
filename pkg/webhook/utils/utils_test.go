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

package utils

import (
	"testing"

	"cuelang.org/go/cue/errors"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
)

func TestValidateCueTemplate(t *testing.T) {
	cases := map[string]struct {
		cueTemplate string
		want        error
	}{
		"normalCueTemp": {
			cueTemplate: "name: 'name'",
			want:        nil,
		},
		"contextNouFoundCueTemp": {
			cueTemplate: `
				output: {
					metadata: {
						name: context.name
						label: context.label
						annotation: "default"
					}
				}`,
			want: nil,
		},
		"inValidCueTemp": {
			cueTemplate: `
				output: {
					metadata: {
						name: context.name
						label: context.label
						annotation: "default"
					},
					hello: world 
				}`,
			want: errors.New("output.hello: reference \"world\" not found"),
		},
	}

	for caseName, cs := range cases {
		t.Run(caseName, func(t *testing.T) {
			err := ValidateCueTemplate(cs.cueTemplate)
			if diff := cmp.Diff(cs.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nValidateCueTemplate: -want , +got \n%s\n", cs.want, diff)
			}
		})
	}
}
