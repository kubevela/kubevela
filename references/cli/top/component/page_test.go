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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPages(t *testing.T) {
	pages := NewPages()
	table := NewTable(&themeConfig)

	t.Run("init", func(t *testing.T) {
		pages.Init()
		pages.Start()
		pages.Stop()
		assert.Equal(t, pages.Stack.Empty(), true)
	})
	t.Run("name", func(t *testing.T) {
		assert.Equal(t, pages.Name(), "Page")
	})
	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(pages.Hint()), 0)
	})

	t.Run("component id", func(t *testing.T) {
		assert.Contains(t, componentID(table), "table")
	})
	t.Run("stack push", func(t *testing.T) {
		pages.StackPush(nil, table)
		assert.Equal(t, pages.HasPage(componentID(table)), true)
	})
	t.Run("stack pop", func(t *testing.T) {
		pages.StackPop(table, nil)
		assert.Equal(t, pages.HasPage(componentID(table)), false)
	})
}
