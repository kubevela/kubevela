/*
Copyright 2022 The KubeVela Authors.

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

package model

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
)

func TestKeyActions_Add(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Add(KeyActions{tcell.KeyTAB: KeyAction{}})
	assert.Equal(t, len(actions), 2)
}

func TestKeyActions_Clear(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Clear()
	assert.Equal(t, len(actions), 0)
}

func TestKeyActions_Delete(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Delete([]tcell.Key{tcell.KeyEnter})
	assert.Equal(t, len(actions), 0)
}

func TestKeyActions_Hint(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	hints := actions.Hint()
	assert.Equal(t, len(hints), 1)
	assert.Equal(t, hints[0], MenuHint{
		Key:         "Enter",
		Description: "",
	})
}

func TestKeyActions_Set(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Set(KeyActions{tcell.KeyHelp: KeyAction{
		Description: "",
		Action:      nil,
		Visible:     false,
		Shared:      false,
	}})
	assert.Equal(t, len(actions), 2)
}
