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
	"strings"
)

// GetBoxDrawingString get line drawing string, see https://en.wikipedia.org/wiki/Box-drawing_character
// nolint:gocyclo
func GetBoxDrawingString(up bool, down bool, left bool, right bool, padLeft int, padRight int) string {
	var c rune
	switch {
	case up && down && left && right:
		c = '┼'
	case up && down && left && !right:
		c = '┤'
	case up && down && !left && right:
		c = '├'
	case up && down && !left && !right:
		c = '│'
	case up && !down && left && right:
		c = '┴'
	case up && !down && left && !right:
		c = '┘'
	case up && !down && !left && right:
		c = '└'
	case up && !down && !left && !right:
		c = '╵'
	case !up && down && left && right:
		c = '┬'
	case !up && down && left && !right:
		c = '┐'
	case !up && down && !left && right:
		c = '┌'
	case !up && down && !left && !right:
		c = '╷'
	case !up && !down && left && right:
		c = '─'
	case !up && !down && left && !right:
		c = '╴'
	case !up && !down && !left && right:
		c = '╶'
	case !up && !down && !left && !right:
		c = ' '
	}
	sb := strings.Builder{}
	writePadding := func(connect bool, width int) {
		for i := 0; i < width; i++ {
			if connect {
				sb.WriteRune('─')
			} else {
				sb.WriteRune(' ')
			}
		}
	}
	writePadding(left, padLeft)
	sb.WriteRune(c)
	writePadding(right, padRight)
	return sb.String()
}

// Sanitize the inputs by removing line endings
func Sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// IgnoreVPrefix removes the leading "v" from a string
func IgnoreVPrefix(s string) string {
	return strings.TrimPrefix(s, "v")
}
