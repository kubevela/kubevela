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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogo(t *testing.T) {
	logo := NewLogo(&themeConfig)
	assert.NotEmpty(t, logo)
	assert.Equal(t, logo.GetText(false),
		` _  __       _          __     __     _        
| |/ /_   _ | |__    ___\ \   / /___ | |  __ _ 
| ' /| | | || '_ \  / _ \\ \ / // _ \| | / _\ |
| . \| |_| || |_) ||  __/ \ V /|  __/| || (_| |
|_|\_\\__,_||_.__/  \___|  \_/  \___||_| \__,_|

`)
}
