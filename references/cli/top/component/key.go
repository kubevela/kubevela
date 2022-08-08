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

import "github.com/gdamore/tcell/v2"

const (
	// KeyHelp corresponding value of keyboard key "?"
	KeyHelp = 63
	// KeySlash corresponding value of keyboard key "/"
	KeySlash = 47
	// KeyColon corresponding value of keyboard key ":"
	KeyColon = 58
	// KeySpace corresponding value of keyboard key "SPACE"
	KeySpace = 32
)

func init() {
	tcell.KeyNames[tcell.Key(KeyHelp)] = "?"
	tcell.KeyNames[tcell.Key(KeySlash)] = "/"
	tcell.KeyNames[tcell.Key(KeySpace)] = "space"
}

// StandardizeKey standardized combined key event and return corresponding key
func StandardizeKey(event *tcell.EventKey) tcell.Key {
	if event.Key() == 256 {
		return tcell.Key(event.Rune())
	}
	return event.Key()
}
