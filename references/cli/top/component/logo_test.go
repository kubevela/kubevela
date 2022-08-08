package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogo(t *testing.T) {
	logo := NewLogo()
	assert.NotEmpty(t, logo)
	assert.Equal(t, logo.GetText(false),
		` _  __       _          __     __     _        
| |/ /_   _ | |__    ___\ \   / /___ | |  __ _ 
| ' /| | | || '_ \  / _ \\ \ / // _ \| | / _\ |
| . \| |_| || |_) ||  __/ \ V /|  __/| || (_| |
|_|\_\\__,_||_.__/  \___|  \_/  \___||_| \__,_|

`)
}
