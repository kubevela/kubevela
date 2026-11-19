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
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/stretchr/testify/assert"
)

func TestString(t *testing.T) {
	testCases := map[string]struct {
		color    Color
		expected string
	}{
		"named color": {
			color:    "red",
			expected: "#ff0000",
		},
		"hex color": {
			color:    "#aabbcc",
			expected: "#aabbcc",
		},
		"default color": {
			color:    DefaultColor,
			expected: "-",
		},
		"invalid color": {
			color:    "invalidColor",
			expected: "-",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.color.String())
		})
	}
}

func TestColor(t *testing.T) {
	c1 := Color("#ff0000")
	assert.Equal(t, tcell.GetColor("#ff0000"), c1.Color())
	c2 := Color("red")
	assert.Equal(t, tcell.GetColor("red").TrueColor(), c2.Color())
	c3 := Color(DefaultColor)
	assert.Equal(t, tcell.ColorDefault, c3.Color())
}

func TestIsHex(t *testing.T) {
	testCases := map[string]struct {
		color    Color
		expected bool
	}{
		"is hex": {
			color:    "#aabbcc",
			expected: true,
		},
		"not hex (too short)": {
			color:    "#aabbc",
			expected: false,
		},
		"not hex (too long)": {
			color:    "#aabbccd",
			expected: false,
		},
		"not hex (no #)": {
			color:    "aabbcc",
			expected: false,
		},
		"not hex (named color)": {
			color:    "red",
			expected: false,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.color.isHex())
		})
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := defaultTheme()
	assert.NotNil(t, theme)
	assert.Equal(t, Color("royalblue"), theme.Info.Title)
	assert.Equal(t, Color("green"), theme.Status.Healthy)
	assert.Equal(t, Color("red"), theme.Status.UnHealthy)
}

func TestPersistentThemeConfig(t *testing.T) {
	defer PersistentThemeConfig(DefaultTheme)
	PersistentThemeConfig("foo")
	bytes, err := os.ReadFile(themeConfigFilePath)
	assert.Nil(t, err)
	assert.True(t, strings.Contains(string(bytes), "foo"))
}

func TestMakeThemeConfigFileIfNotExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vela-theme-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	originalHomePath := homePath
	originalThemeConfigFilePath := themeConfigFilePath
	homePath = tmpDir
	themeConfigFilePath = filepath.Join(tmpDir, themeHomeDirPath, themeConfigFile)
	defer func() {
		homePath = originalHomePath
		themeConfigFilePath = originalThemeConfigFilePath
	}()

	t.Run("should create file if it does not exist", func(t *testing.T) {
		os.Remove(themeConfigFilePath)

		exists := makeThemeConfigFileIfNotExist()
		assert.False(t, exists, "should return false as file was created")

		_, err := os.Stat(themeConfigFilePath)
		assert.NoError(t, err, "expected theme config file to be created")
		content, err := os.ReadFile(themeConfigFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "name : "+DefaultTheme, string(content))
	})

	t.Run("should not modify file if it already exists", func(t *testing.T) {
		customContent := "name : custom"
		err := os.WriteFile(themeConfigFilePath, []byte(customContent), 0600)
		assert.NoError(t, err)

		exists := makeThemeConfigFileIfNotExist()
		assert.True(t, exists, "should return true as file already exists")

		content, err := os.ReadFile(themeConfigFilePath)
		assert.NoError(t, err)
		assert.Equal(t, customContent, string(content))
	})
}

func TestLoadThemeConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vela-theme-test-load")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	originalHomePath := homePath
	originalThemeConfigFilePath := themeConfigFilePath
	originalDiyThemeDirPath := diyThemeDirPath
	homePath = tmpDir
	themeConfigFilePath = filepath.Join(tmpDir, themeHomeDirPath, themeConfigFile)
	diyThemeDirPath = filepath.Join(tmpDir, themeHomeDirPath, diyThemeDir)
	defer func() {
		homePath = originalHomePath
		themeConfigFilePath = originalThemeConfigFilePath
		diyThemeDirPath = originalDiyThemeDirPath
	}()

	ThemeMap["custom"] = ThemeConfig{
		Info: struct {
			Title Color `yaml:"title"`
			Text  Color `yaml:"text"`
		}{Title: "custom-title"},
	}
	defer delete(ThemeMap, "custom")

	t.Run("config file not exist", func(t *testing.T) {
		os.Remove(themeConfigFilePath)
		cfg := LoadThemeConfig()
		assert.Equal(t, defaultTheme().Info.Title, cfg.Info.Title)
	})

	t.Run("config file with default theme", func(t *testing.T) {
		PersistentThemeConfig(DefaultTheme)
		cfg := LoadThemeConfig()
		assert.Equal(t, defaultTheme().Info.Title, cfg.Info.Title)
	})

	t.Run("config file with custom theme", func(t *testing.T) {
		PersistentThemeConfig("custom")
		cfg := LoadThemeConfig()
		assert.Equal(t, Color("custom-title"), cfg.Info.Title)
	})

	t.Run("config file with unknown theme", func(t *testing.T) {
		PersistentThemeConfig("unknown")
		cfg := LoadThemeConfig()
		assert.Equal(t, defaultTheme().Info.Title, cfg.Info.Title)
	})

	t.Run("config file with invalid content", func(t *testing.T) {
		err := os.WriteFile(themeConfigFilePath, []byte("name: [invalid"), 0600)
		assert.NoError(t, err)
		cfg := LoadThemeConfig()
		assert.Equal(t, defaultTheme().Info.Title, cfg.Info.Title)
	})
}
