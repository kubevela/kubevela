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

// Defines char keystrokes.
const (
	KeyA tcell.Key = iota + 97
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
)

func init() {
	tcell.KeyNames[tcell.Key(KeyHelp)] = "?"
	tcell.KeyNames[tcell.Key(KeySlash)] = "/"
	tcell.KeyNames[tcell.Key(KeySpace)] = "space"

	initStdKeys()
}

func initStdKeys() {
	tcell.KeyNames[KeyA] = "a"
	tcell.KeyNames[KeyB] = "b"
	tcell.KeyNames[KeyC] = "c"
	tcell.KeyNames[KeyD] = "d"
	tcell.KeyNames[KeyE] = "e"
	tcell.KeyNames[KeyF] = "f"
	tcell.KeyNames[KeyG] = "g"
	tcell.KeyNames[KeyH] = "h"
	tcell.KeyNames[KeyI] = "i"
	tcell.KeyNames[KeyJ] = "j"
	tcell.KeyNames[KeyK] = "k"
	tcell.KeyNames[KeyL] = "l"
	tcell.KeyNames[KeyM] = "m"
	tcell.KeyNames[KeyN] = "n"
	tcell.KeyNames[KeyO] = "o"
	tcell.KeyNames[KeyP] = "p"
	tcell.KeyNames[KeyQ] = "q"
	tcell.KeyNames[KeyR] = "r"
	tcell.KeyNames[KeyS] = "s"
	tcell.KeyNames[KeyT] = "t"
	tcell.KeyNames[KeyU] = "u"
	tcell.KeyNames[KeyV] = "v"
	tcell.KeyNames[KeyW] = "w"
	tcell.KeyNames[KeyX] = "x"
	tcell.KeyNames[KeyY] = "y"
	tcell.KeyNames[KeyZ] = "z"
}

// StandardizeKey standardized combined key event and return corresponding key
func StandardizeKey(event *tcell.EventKey) tcell.Key {
	if event.Key() == 256 {
		return tcell.Key(event.Rune())
	}
	return event.Key()
}
