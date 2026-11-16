/*
 Copyright 2026. The KubeVela Authors.

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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/cue/nesting"
)

// An addon's CUE nested too deeply for CUE's parser is refused before it is parsed.
func TestAnAddonNestedTooDeeplyIsRefused(t *testing.T) {
	deep := "output: " + strings.Repeat("[", nesting.MaxDepth+1)
	_, err := newValueWithMainAndFiles(deep, nil, "")
	require.ErrorContains(t, err, "nested deeper than 1000 levels")
	_, err = newValueWithMainAndFiles("package main\noutput: {}", []string{deep}, "")
	require.ErrorContains(t, err, "nested deeper than 1000 levels")
	_, err = checkCueFileHasPackageHeader(ElementFile{Name: "deep.cue", Data: deep})
	require.ErrorContains(t, err, "nested deeper than 1000 levels")
}

// So are the files an addon render builds from its parts: the context file from its parameters,
// and its notes.
func TestAnAddonsOtherCUEIsCheckedToo(t *testing.T) {
	deep := "x: " + strings.Repeat("[", nesting.MaxDepth+1)
	r := addonCueTemplateRender{addon: &InstallPackage{Meta: Meta{Name: "deep"}, Parameters: deep}}
	_, err := r.formatContext()
	require.ErrorContains(t, err, "nested deeper than 1000 levels")
	var comp map[string]interface{}
	require.ErrorContains(t, r.toObject("output: {}", "output", &comp), "nested deeper than 1000 levels")

	h := &Installer{}
	_, err = h.renderNotes(&InstallPackage{Meta: Meta{Name: "deep"}, Notes: ElementFile{Name: "NOTES.cue", Data: deep}})
	require.ErrorContains(t, err, "nested deeper than 1000 levels")

}
