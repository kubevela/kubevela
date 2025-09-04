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

package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitize(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "with newlines and carriage returns",
			input: "abc\ndef\rgh",
			want:  "abcdefgh",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "no special characters",
			input: "cleanstring",
			want:  "cleanstring",
		},
		{
			name:  "only special characters",
			input: "\n\r\n",
			want:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Sanitize(tc.input)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIgnoreVPrefix(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "with lowercase v prefix",
			input: "v1.2.0",
			want:  "1.2.0",
		},
		{
			name:  "without v prefix",
			input: "1.2.0",
			want:  "1.2.0",
		},
		{
			name:  "with uppercase V prefix",
			input: "V1.3.0",
			want:  "V1.3.0",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "string is just v",
			input: "v",
			want:  "",
		},
		{
			name:  "string is just V",
			input: "V",
			want:  "V",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IgnoreVPrefix(tc.input)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetBoxDrawingString_Chars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		up, down    bool
		left, right bool
		want        rune
	}{
		{"cross ┼", true, true, true, true, '┼'},
		{"up down left ┤", true, true, true, false, '┤'},
		{"up down right ├", true, true, false, true, '├'},
		{"up down only │", true, true, false, false, '│'},
		{"up left right ┴", true, false, true, true, '┴'},
		{"up left ┘", true, false, true, false, '┘'},
		{"up right └", true, false, false, true, '└'},
		{"up only ╵", true, false, false, false, '╵'},
		{"down left right ┬", false, true, true, true, '┬'},
		{"down left ┐", false, true, true, false, '┐'},
		{"down right ┌", false, true, false, true, '┌'},
		{"down only ╷", false, true, false, false, '╷'},
		{"left right ─", false, false, true, true, '─'},
		{"left only ╴", false, false, true, false, '╴'},
		{"right only ╶", false, false, false, true, '╶'},
		{"none space", false, false, false, false, ' '},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := GetBoxDrawingString(tc.up, tc.down, tc.left, tc.right, 0, 0)
			require.Equal(t, string(tc.want), got)
		})
	}
}

func TestGetBoxDrawingString_Padding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                  string
		up, down, left, right bool
		padLeft, padRight     int
		center                rune
		leftPadChar           rune
		rightPadChar          rune
	}{
		{"no connectors -> spaces", false, false, false, false, 2, 3, ' ', ' ', ' '},
		{"left connector pads with lines", false, false, true, false, 2, 1, '╴', '─', ' '},
		{"right connector pads with lines", false, false, false, true, 1, 2, '╶', ' ', '─'},
		{"both connectors pads with lines", false, false, true, true, 2, 2, '─', '─', '─'},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := GetBoxDrawingString(tc.up, tc.down, tc.left, tc.right, tc.padLeft, tc.padRight)
			leftPad := strings.Repeat(string(tc.leftPadChar), tc.padLeft)
			rightPad := strings.Repeat(string(tc.rightPadChar), tc.padRight)
			expected := leftPad + string(tc.center) + rightPad
			require.Equal(t, expected, got)
		})
	}
}
