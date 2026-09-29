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
	r.Equal("test/app", AddSharer("", appKey(app)))
	r.Equal("test/app,x/y", AddSharer("test/app,x/y", appKey(app)))
	r.Equal("x/y,test/app", AddSharer("x/y", appKey(app)))
	r.True(ContainsSharer("a/b,test/app,x/y", appKey(app)))
	r.False(ContainsSharer("a/b,x/y", appKey(app)))
	r.Equal("a/b", FirstSharer("a/b,x/y"))
	r.Equal("", FirstSharer(""))
	r.Equal("a/b,x/y", RemoveSharer("a/b,test/app,x/y", appKey(app)))
	r.Equal("a/b,x/y", RemoveSharer("a/b,x/y", appKey(app)))
}

// Owner keys have a canonical form, <kind>/<namespace>/<name>. Keys written before kinds
// existed (<namespace>/<name>, or <name> in the default namespace) belong to the configured
// default kind, and every sharer operation treats both forms of one owner as the same owner,
// so lists mixing them neither duplicate nor leak an entry.
func TestSharersCompareOwnerKeysCanonically(t *testing.T) {
	prev := CurrentConfig()
	defer Configure(prev)
	Configure(Config{DefaultOwnerKind: "Application"})
	r := require.New(t)

	r.Equal("Application/team-a/shop", CanonicalOwnerKey("team-a/shop"))
	r.Equal("Application/default/shop", CanonicalOwnerKey("shop"))
	r.Equal("Component/team-a/backend", CanonicalOwnerKey("Component/team-a/backend"))

	r.True(ContainsSharer("Application/team-a/shop", "team-a/shop"), "old form finds the canonical entry")
	r.True(ContainsSharer("team-a/shop", "Application/team-a/shop"), "canonical form finds the old entry")
	r.False(ContainsSharer("Component/team-a/shop", "team-a/shop"), "same name, different kind")

	r.Equal("Application/team-a/shop", AddSharer("Application/team-a/shop", "team-a/shop"), "no duplicate across forms")
	r.Equal("x/y,team-a/shop", AddSharer("x/y", "team-a/shop"), "a new sharer is written in the form given")

	r.Equal("Component/team-a/backend", RemoveSharer("team-a/shop,Application/team-a/shop,Component/team-a/backend", "team-a/shop"), "both forms of the leaver go")
	r.Equal("x/y", RemoveSharer("default/shop,x/y", "shop"), "a bare name is the default namespace")
}

// Without a default kind, kindless keys are left as they are and compared as strings.
func TestSharersWithoutADefaultKind(t *testing.T) {
	prev := CurrentConfig()
	defer Configure(prev)
	Configure(Config{})
	require.Equal(t, "team-a/shop", CanonicalOwnerKey("team-a/shop"))
	require.False(t, ContainsSharer("Application/team-a/shop", "team-a/shop"))
}

// A shared-by list can hold an empty entry: a legacy annotation, or one left by removing a
// sharer. Handing the resource to the next sharer must not pick it.
func TestFirstSharerSkipsEmptyEntries(t *testing.T) {
	require.Equal(t, "team-a/shop", FirstSharer(",team-a/shop"))
	require.Equal(t, "team-a/shop", FirstSharer("team-a/shop,team-b/other"))
	require.Empty(t, FirstSharer(""))
	require.Empty(t, FirstSharer(","))
	require.Equal(t, "team-b/other", RemoveSharer(",team-a/shop,team-b/other", "team-a/shop"),
		"removing a sharer drops the empty entries with it")
}
