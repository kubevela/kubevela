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

package cachekey

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRulesReturnsTheCurrentPolicy(t *testing.T) {
	rules, err := LoadRules()
	require.NoError(t, err)
	require.NotEmpty(t, rules.Hash, "the hash is stamped on every generated definition")
	require.NotEmpty(t, rules.Version, "the readable name travels with the hash")
	require.NotEmpty(t, rules.Fields(), "rules that key on nothing would give every source one entry")
}

// The rules are embedded, so a file's content cannot change while the process
// runs. Parsing was most of what a render spent on an already-cached source,
// and the memoised value is shared rather than copied.
func TestLoadRulesIsMemoisedAndShared(t *testing.T) {
	first, err := LoadRules()
	require.NoError(t, err)
	second, err := LoadRules()
	require.NoError(t, err)
	require.Same(t, first, second, "the parsed rules are shared, not reparsed")
}

func TestLoadRulesIsSafeUnderConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := LoadRules()
			require.NoError(t, err)
			require.NotEmpty(t, r.Fields())
		}()
	}
	wg.Wait()
}

// A definition records the rules it was generated against so it keeps validating
// after the policy moves on.
func TestLoadRulesByHashFindsTheRecordedPolicy(t *testing.T) {
	current, err := LoadRules()
	require.NoError(t, err)

	got, err := LoadRulesByHash(current.Hash)
	require.NoError(t, err)
	require.Equal(t, current.Hash, got.Hash)
	require.Equal(t, current.Fields(), got.Fields())
}

// A hash this build does not carry is an error, not a fallback to the current
// rules: validating against different rules than the ones used to generate
// would defeat the point of recording it.
func TestLoadRulesByHashRefusesAnUnknownPolicy(t *testing.T) {
	_, err := LoadRulesByHash("deadbeef")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not present",
		"the message has to say the build lacks them, not that the definition is wrong")
}

func TestRuleFileNamesAreSortedSoTheLatestIsLast(t *testing.T) {
	names, err := ruleFileNames()
	require.NoError(t, err)
	require.NotEmpty(t, names)
	require.IsIncreasing(t, names, "LoadRules takes the last, so the order is the policy order")
	for _, n := range names {
		require.Contains(t, n, "rules/")
	}
}

// The hash names the content. If it were computed over anything that varies
// between builds, every definition would need regenerating on every release.
func TestPolicyHashIsStableAcrossCalls(t *testing.T) {
	rules, err := LoadRules()
	require.NoError(t, err)
	first, err := rules.policyHash()
	require.NoError(t, err)
	second, err := rules.policyHash()
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, rules.Hash, first, "the stamped hash is the computed one")
}

func TestFieldsAndKeyedEntryAgree(t *testing.T) {
	rules, err := LoadRules()
	require.NoError(t, err)
	for _, f := range rules.Fields() {
		_, ok := rules.keyedEntry(f)
		require.True(t, ok, "%s is listed by Fields, so it must have an entry", f)
	}
	_, ok := rules.keyedEntry("notAFieldAnyRuleKeysOn")
	require.False(t, ok)
}
