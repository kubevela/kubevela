/*
Copyright 2026 The KubeVela Authors.

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

package propexpr

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJoinAndIndexPath(t *testing.T) {
	require.Equal(t, "host", JoinPath("", "host"))
	require.Equal(t, "db.host", JoinPath("db", "host"))
	require.Equal(t, "tags[0]", IndexPath("tags", 0))
	require.Equal(t, "db.tags[2]", IndexPath("db.tags", 2))
}

// The path a leaf is reported at is the same string validation puts in a field
// error and a render records in status, so both name the same place.
func TestWalkReportsEveryStringLeafWithItsPath(t *testing.T) {
	var seen []string
	err := Walk(map[string]interface{}{
		"host": "example.com",
		"port": 8080,
		"tls":  true,
		"tags": []interface{}{"a", "b"},
		"db": map[string]interface{}{
			"user":  "admin",
			"hosts": []interface{}{map[string]interface{}{"name": "primary"}},
		},
		"nothing": nil,
	}, "", func(path, raw string) error {
		seen = append(seen, path+"="+raw)
		return nil
	})
	require.NoError(t, err)
	sort.Strings(seen)
	require.Equal(t, []string{
		"db.hosts[0].name=primary",
		"db.user=admin",
		"host=example.com",
		"tags[0]=a",
		"tags[1]=b",
	}, seen, "non-string leaves are skipped; an expression is always a string")
}

func TestWalkStopsAtTheFirstError(t *testing.T) {
	stop := errors.New("stop")
	calls := 0
	err := Walk(map[string]interface{}{
		"a": "1", "b": "2", "c": "3",
	}, "", func(string, string) error {
		calls++
		return stop
	})
	require.ErrorIs(t, err, stop)
	require.Equal(t, 1, calls, "the walk ends rather than collecting every leaf")
}

func TestWalkOnScalarsAndEmpties(t *testing.T) {
	var seen []string
	require.NoError(t, Walk("bare", "root", func(p, r string) error {
		seen = append(seen, p+"="+r)
		return nil
	}))
	require.Equal(t, []string{"root=bare"}, seen, "a bare string is itself a leaf")

	require.NoError(t, Walk(nil, "", func(string, string) error {
		t.Fatal("nil has no leaves")
		return nil
	}))
	require.NoError(t, Walk(42, "", func(string, string) error {
		t.Fatal("a number is not a leaf this visits")
		return nil
	}))
}

// Map builds a new tree. The binding properties on a render context are read by
// every component and trait of an Application, so rewriting them in place would
// make the second read see something different from the first.
func TestMapLeavesItsInputAlone(t *testing.T) {
	in := map[string]interface{}{
		"host": "$(source.cfg.host)",
		"port": 8080,
		"tags": []interface{}{"$(source.cfg.tag)", "literal"},
	}

	out, err := Map(in, "", func(_, raw string) (interface{}, error) {
		if strings.HasPrefix(raw, "$(") {
			return "resolved", nil
		}
		return raw, nil
	})
	require.NoError(t, err)

	require.Equal(t, map[string]interface{}{
		"host": "resolved",
		"port": 8080,
		"tags": []interface{}{"resolved", "literal"},
	}, out)
	require.Equal(t, map[string]interface{}{
		"host": "$(source.cfg.host)",
		"port": 8080,
		"tags": []interface{}{"$(source.cfg.tag)", "literal"},
	}, in, "the input still reads as it did, so a second render sees the same thing")
}

// A leaf may become any type, not just a string: an expression resolving to a
// whole struct replaces the string it was written as.
func TestMapCanChangeALeafsType(t *testing.T) {
	out, err := Map(map[string]interface{}{"meta": "$(source.cfg.meta)"}, "",
		func(_, _ string) (interface{}, error) {
			return map[string]interface{}{"region": "eu-west"}, nil
		})
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"meta": map[string]interface{}{"region": "eu-west"},
	}, out)
}

func TestMapPropagatesTheFirstError(t *testing.T) {
	boom := errors.New("boom")
	_, err := Map(map[string]interface{}{"a": map[string]interface{}{"b": "x"}}, "",
		func(string, string) (interface{}, error) { return nil, boom })
	require.ErrorIs(t, err, boom)

	_, err = Map(map[string]interface{}{"list": []interface{}{"x"}}, "",
		func(string, string) (interface{}, error) { return nil, boom })
	require.ErrorIs(t, err, boom)
}

func TestMapPassesThroughWhatItDoesNotVisit(t *testing.T) {
	fail := func(string, string) (interface{}, error) {
		t.Fatal("should not be called")
		return nil, nil
	}
	for _, in := range []interface{}{42, true, nil, 3.5} {
		out, err := Map(in, "", fail)
		require.NoError(t, err)
		require.Equal(t, in, out)
	}
}
