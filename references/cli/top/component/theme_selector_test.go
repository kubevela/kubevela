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

package component

import (
	"fmt"
	"testing"

	"github.com/rivo/tview"
	"github.com/stretchr/testify/assert"
)

func TestThemeSelector(t *testing.T) {
	s := NewThemeSelector(&themeConfig, nil)

	assert.Equal(t, s.Frame.GetPrimitive().(*tview.List), s.list)

	t.Run("init", func(t *testing.T) {
		s.Init()
		assert.Equal(t, s.Frame.GetBorderColor(), themeConfig.Border.Table.Color())
		assert.Equal(t, s.Frame.GetTitle(), fmt.Sprintf("[ %s ]", "Select Theme"))
	})

	t.Run("start", func(t *testing.T) {
		s.Start()
		assert.Equal(t, s.list.GetItemCount() >= 14, true)
	})
}
