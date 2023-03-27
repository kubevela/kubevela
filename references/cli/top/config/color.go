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
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"gopkg.in/yaml.v3"
)

// Color is a color string.
type Color string

// ThemeConfig is the theme config.
type ThemeConfig struct {
	Info struct {
		Title Color
		Text  Color
	}
	Menu struct {
		Description Color
		Key         Color
	}
	Logo struct {
		Text Color
	}
	Crumbs struct {
		Foreground Color
		Background Color
	}
	Border struct {
		App   Color
		Table Color
	}
	Table struct {
		Title    Color
		Header   Color
		Body     Color
		CursorBg Color
		CursorFg Color
	}
	Status struct {
		Starting  Color
		Healthy   Color
		UnHealthy Color
		Waiting   Color
		Succeeded Color
		Failed    Color
		Unknown   Color
	}
	Yaml struct {
		Key   Color
		Colon Color
		Value Color
	}
	Topology struct {
		Line      Color
		App       Color
		Workflow  Color
		Component Color
		Policy    Color
		Trait     Color
		Kind      Color
	}
}

var (
	// ThemeConfigFS is the theme config file system.
	//go:embed theme/*
	ThemeConfigFS embed.FS
	// ThemeMap is the theme map.
	ThemeMap = make(map[string]ThemeConfig)
	// ThemeNameArray is the theme name array.
	ThemeNameArray []string
	// homePath is the home path.
	homePath string
	// diyThemeDirPath is the diy theme dir path like ~/.vela/theme/themes
	diyThemeDirPath string
	// themeConfigFilePath is the theme config file path like ~/.vela/theme/_config.yaml
	themeConfigFilePath string
)

const (
	// DefaultColor represents a default color.
	DefaultColor Color = "default"
	// DefaultTheme represents a default theme.
	DefaultTheme     = "default"
	embedThemePath   = "theme"
	themeHomeDirPath = ".vela/theme"
	diyThemeDir      = "themes"
	themeConfigFile  = "_config.yaml"
)

func init() {
	homePath, _ = os.UserHomeDir()
	diyThemeDirPath = filepath.Join(homePath, themeHomeDirPath, diyThemeDir)
	themeConfigFilePath = filepath.Join(homePath, themeHomeDirPath, themeConfigFile)

	dir, err := ThemeConfigFS.ReadDir(embedThemePath)
	if err != nil {
		return
	}

	// embed theme config
	for _, item := range dir {
		content, err := ThemeConfigFS.ReadFile(filepath.Join(embedThemePath, item.Name()))
		if err != nil {
			continue
		}
		var t ThemeConfig
		err = yaml.Unmarshal(content, &t)
		if err != nil {
			continue
		}
		themeName := strings.Split(item.Name(), ".")[0]
		ThemeMap[themeName] = t
		ThemeNameArray = append(ThemeNameArray, themeName)
	}

	// load diy theme config
	dir, err = os.ReadDir(diyThemeDirPath)
	if err != nil {
		return
	}
	for _, item := range dir {
		content, err := os.ReadFile(filepath.Clean(filepath.Join(diyThemeDirPath, item.Name())))
		if err != nil {
			continue
		}
		var t ThemeConfig
		err = yaml.Unmarshal(content, &t)
		if err != nil {
			continue
		}
		themeName := strings.Split(item.Name(), ".")[0]
		ThemeMap[themeName] = t
		ThemeNameArray = append(ThemeNameArray, themeName)
	}
}

// LoadThemeConfig loads theme config from env or use the default setting
func LoadThemeConfig() *ThemeConfig {
	themeConfigName := struct {
		Name string `yaml:"name"`
	}{}
	// returns default theme if config file not exist
	if !makeThemeConfigFileIfNotExist() {
		return defaultTheme()
	}

	content, err := os.ReadFile(filepath.Clean(themeConfigFilePath))
	if err != nil {
		return defaultTheme()
	}
	err = yaml.Unmarshal(content, &themeConfigName)
	if err != nil {
		return defaultTheme()
	}
	if themeConfigName.Name == DefaultTheme {
		return defaultTheme()
	}

	if config, ok := ThemeMap[themeConfigName.Name]; ok {
		return &config
	}
	return defaultTheme()
}

func defaultTheme() *ThemeConfig {
	return &ThemeConfig{
		Info: struct {
			Title Color
			Text  Color
		}{
			Title: "royalblue",
			Text:  "lightgray",
		},
		Menu: struct {
			Description Color
			Key         Color
		}{
			Description: "gray",
			Key:         "royalblue",
		},
		Logo: struct {
			Text Color
		}{
			Text: "royalblue",
		},
		Crumbs: struct {
			Foreground Color
			Background Color
		}{
			Foreground: "white",
			Background: "royalblue",
		},
		Border: struct {
			App   Color
			Table Color
		}{
			App:   "black",
			Table: "lightgray",
		},
		Table: struct {
			Title    Color
			Header   Color
			Body     Color
			CursorBg Color
			CursorFg Color
		}{
			Title:    "royalblue",
			Header:   "white",
			Body:     "blue",
			CursorBg: "blue",
			CursorFg: "black",
		},
		Yaml: struct {
			Key   Color
			Colon Color
			Value Color
		}{
			Key:   "#d33582",
			Colon: "lightgray",
			Value: "#839495",
		},
		Status: struct {
			Starting  Color
			Healthy   Color
			UnHealthy Color
			Waiting   Color
			Succeeded Color
			Failed    Color
			Unknown   Color
		}{
			Starting:  "blue",
			Healthy:   "green",
			UnHealthy: "red",
			Waiting:   "yellow",
			Succeeded: "orange",
			Failed:    "purple",
			Unknown:   "gray",
		},
		Topology: struct {
			Line      Color
			App       Color
			Workflow  Color
			Component Color
			Policy    Color
			Trait     Color
			Kind      Color
		}{
			Line:      "cadetblue",
			App:       "red",
			Workflow:  "orange",
			Component: "green",
			Policy:    "yellow",
			Trait:     "lightseagreen",
			Kind:      "orange",
		},
	}
}

// PersistentThemeConfig saves theme config to file
func PersistentThemeConfig(themeName string) {
	makeThemeConfigFileIfNotExist()
	_ = os.WriteFile(themeConfigFilePath, []byte("name : "+themeName), 0600)
}

// makeThemeConfigFileIfNotExist makes theme config file and write default content if not exist
func makeThemeConfigFileIfNotExist() bool {
	velaThemeHome := filepath.Clean(filepath.Join(homePath, themeHomeDirPath))
	if _, err := os.Open(filepath.Clean(themeConfigFilePath)); err != nil {
		if os.IsNotExist(err) {
			// make file if not exist
			_ = os.MkdirAll(filepath.Clean(velaThemeHome), 0700)
			_ = os.WriteFile(filepath.Clean(themeConfigFilePath), []byte("name : "+DefaultTheme), 0600)
		}
		return false
	}
	return true
}

// String returns color as string.
func (c Color) String() string {
	if c.isHex() {
		return string(c)
	}
	if c == DefaultColor {
		return "-"
	}
	col := c.Color().TrueColor().Hex()
	if col < 0 {
		return "-"
	}

	return fmt.Sprintf("#%06x", col)
}

func (c Color) isHex() bool {
	return len(c) == 7 && c[0] == '#'
}

// Color returns a view color.
func (c Color) Color() tcell.Color {
	if c == DefaultColor {
		return tcell.ColorDefault
	}

	return tcell.GetColor(string(c)).TrueColor()
}
