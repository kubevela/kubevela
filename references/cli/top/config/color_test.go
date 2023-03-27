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

package config

import (
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	c := Color("red")
	assert.Equal(t, c.String(), "#ff0000")
}

func TestColor(t *testing.T) {
	c1 := Color("#ff0000")
	assert.Equal(t, c1.Color(), tcell.GetColor("#ff0000"))
	c2 := Color("red")
	assert.Equal(t, c2.Color(), tcell.GetColor("red").TrueColor())
}

func TestPersistentThemeConfig(t *testing.T) {
	defer PersistentThemeConfig(DefaultTheme)
	PersistentThemeConfig("foo")
	bytes, err := os.ReadFile(themeConfigFilePath)
	assert.Nil(t, err)
	assert.True(t, strings.Contains(string(bytes), "foo"))
}
