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
