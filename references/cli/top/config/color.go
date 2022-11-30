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
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
	"gopkg.in/yaml.v3"
)

type Color string

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

const (
	// DefaultColor represents a default color.
	DefaultColor Color = "default"
)

func LoadThemeConfig() *ThemeConfig {
	if theme, ok := haveThemeSetting(); !ok {
		return defaultTheme()
	} else {
		return theme
	}
}

func haveThemeSetting() (*ThemeConfig, bool) {
	themeFile := os.Getenv("VELA_TOP_THEME")
	if len(themeFile) == 0 {
		return nil, false
	}
	content, err := os.ReadFile(themeFile)
	if err != nil {
		return nil, false
	}
	t := new(ThemeConfig)
	err = yaml.Unmarshal(content, t)
	if err != nil {
		return nil, false
	}
	return t, true
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
