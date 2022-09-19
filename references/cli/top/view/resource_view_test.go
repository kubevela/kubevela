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

package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
)

func TestResourceView(t *testing.T) {
	app := NewApp(nil, nil, "")
	view := NewCommonView(app)
	assert.Equal(t, view.Name(), "Resource")

	view.Init()
	assert.Equal(t, view.GetBorderColor(), tcell.ColorWhite)
	assert.Equal(t, len(view.Hint()), 2)

	view.BuildHeader([]string{"Name", "Data"})
	assert.Equal(t, view.GetCell(0, 0).Text, "Name")

	view.BuildBody([][]string{{"Name1", "Data1"}})
	assert.Equal(t, view.GetCell(1, 0).Text, "Name1")
	assert.Equal(t, view.GetCell(1, 1).Text, "Data1")

	view.Refresh(true, func() {})
	assert.Equal(t, view.GetCell(0, 0).Text, "")

	view.BuildHeader([]string{"Name", "Data"})
	assert.Equal(t, view.GetCell(0, 0).Text, "Name")

	view.Stop()
	assert.Equal(t, view.GetCell(0, 0).Text, "")

}
