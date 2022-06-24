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

package apply

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestShare(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{}
	app.SetName("app")
	app.SetNamespace("test")
	r.Equal("test/app", AddSharer("", app))
	r.Equal("test/app,x/y", AddSharer("test/app,x/y", app))
	r.Equal("x/y,test/app", AddSharer("x/y", app))
	r.True(ContainsSharer("a/b,test/app,x/y", app))
	r.False(ContainsSharer("a/b,x/y", app))
	r.Equal("a/b", FirstSharer("a/b,x/y"))
	r.Equal("", FirstSharer(""))
	r.Equal("a/b,x/y", RemoveSharer("a/b,test/app,x/y", app))
	r.Equal("a/b,x/y", RemoveSharer("a/b,x/y", app))
}
