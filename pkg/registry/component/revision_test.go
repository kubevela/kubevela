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

package component

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPackageName(t *testing.T) {
	for name, want := range map[string]bool{
		"widget-kit":     true,
		"a":              true,
		"kit_v2":         true,
		"":               false,
		".":              false,
		"..":             false,
		"../etc":         false,
		"kit/sub":        false,
		`kit\sub`:        false,
		"./kit":          false,
		"kit/":           false,
		"kit/../gadget":  false,
		"kit-with.dots":  true,
		"UPPER":          true,
		"with space":     true, // not a path concern; the registry decides
		"trailing-dot./": false,
	} {
		assert.Equal(t, want, IsPackageName(name), "IsPackageName(%q)", name)
	}
}

func TestRateLimitedErrorMessage(t *testing.T) {
	at := time.Date(2026, 10, 4, 12, 30, 0, 0, time.FixedZone("x", 3600))
	cases := []struct {
		name string
		err  RateLimitedError
		want string
	}{
		{"source and reset", RateLimitedError{Source: "ghcr.io", Until: at},
			"ghcr.io: exceed github access rate limit, resets at 2026-10-04T11:30:00Z"},
		{"source only", RateLimitedError{Source: "ghcr.io"},
			"ghcr.io: exceed github access rate limit"},
		{"reset only", RateLimitedError{Until: at},
			"exceed github access rate limit, resets at 2026-10-04T11:30:00Z"},
		{"bare", RateLimitedError{}, "exceed github access rate limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &tc.err
			assert.Equal(t, tc.want, err.Error())
			assert.ErrorIs(t, err, ErrRateLimit, "every rate-limit error must still match the sentinel")
			assert.ErrorIs(t, fmt.Errorf("wrapped: %w", err), ErrRateLimit)
		})
	}
}

func TestRateLimitedErrorRetryAfter(t *testing.T) {
	assert.Equal(t, time.Duration(0), (&RateLimitedError{}).RetryAfter(), "no reset time means ask now")
	assert.Equal(t, time.Duration(0), (&RateLimitedError{Until: time.Now().Add(-time.Minute)}).RetryAfter(),
		"a reset in the past means ask now")
	d := (&RateLimitedError{Until: time.Now().Add(time.Minute)}).RetryAfter()
	assert.Greater(t, d, 50*time.Second)
	assert.LessOrEqual(t, d, time.Minute)
}

func TestRateLimitGateBlocksUntilDeadline(t *testing.T) {
	g := &rateLimitGate{until: map[string]time.Time{}}

	assert.NoError(t, g.blocked(""), "an empty key is never gated")
	assert.NoError(t, g.blocked("ghcr.io"), "an unknown source is worth calling")

	until := time.Now().Add(time.Minute)
	g.trip("ghcr.io", until)
	err := g.blocked("ghcr.io")
	require.Error(t, err)
	var limited *RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, "ghcr.io", limited.Source)
	assert.True(t, limited.Until.Equal(until))
	assert.NoError(t, g.blocked("other.example.com"), "a hold is per source")
}

func TestRateLimitGateForgetsExpiredHold(t *testing.T) {
	g := &rateLimitGate{until: map[string]time.Time{}}
	g.trip("ghcr.io", time.Now().Add(-time.Second))
	assert.NoError(t, g.blocked("ghcr.io"), "a deadline that has passed no longer blocks")
	_, still := g.until["ghcr.io"]
	assert.False(t, still, "an expired hold is dropped on the way out")
}

func TestRateLimitGateTripIgnoresEmptyKey(t *testing.T) {
	g := &rateLimitGate{until: map[string]time.Time{}}
	g.trip("", time.Now().Add(time.Hour))
	assert.Empty(t, g.until)
}

func TestRateLimitGateKeepsTheLaterDeadline(t *testing.T) {
	g := &rateLimitGate{until: map[string]time.Time{}}
	later := time.Now().Add(30 * time.Minute)
	g.trip("ghcr.io", later)
	g.trip("ghcr.io", time.Now().Add(time.Minute))
	assert.True(t, g.until["ghcr.io"].Equal(later), "a shorter refusal must not cut an existing hold short")

	evenLater := time.Now().Add(40 * time.Minute)
	g.trip("ghcr.io", evenLater)
	assert.True(t, g.until["ghcr.io"].Equal(evenLater), "a longer refusal extends the hold")
}

func TestRateLimitGateCapsTheHold(t *testing.T) {
	g := &rateLimitGate{until: map[string]time.Time{}}
	before := time.Now()
	g.trip("ghcr.io", before.Add(365*24*time.Hour))
	got := g.until["ghcr.io"]
	assert.True(t, got.After(before.Add(maxRateLimitHold-time.Second)), "the hold must be about an hour")
	assert.True(t, got.Before(time.Now().Add(maxRateLimitHold+time.Second)), "a year-long Retry-After is clamped to an hour")
}

func TestResetRateLimitGate(t *testing.T) {
	ResetRateLimitGate()
	sourceRateLimit.trip("ghcr.io", time.Now().Add(time.Hour))
	require.Error(t, sourceRateLimit.blocked("ghcr.io"))
	ResetRateLimitGate()
	assert.NoError(t, sourceRateLimit.blocked("ghcr.io"))
}

func TestRateLimitHold(t *testing.T) {
	until := time.Now().Add(time.Minute)
	got, ok := rateLimitHold(fmt.Errorf("render: %w", &RateLimitedError{Source: "ghcr.io", Until: until}))
	assert.True(t, ok)
	assert.True(t, got.Equal(until), "a re-wrapped refusal keeps its original deadline")

	_, ok = rateLimitHold(errors.New("not a rate limit"))
	assert.False(t, ok)
	_, ok = rateLimitHold(ErrRateLimit)
	assert.False(t, ok, "the bare sentinel carries no deadline")
}
