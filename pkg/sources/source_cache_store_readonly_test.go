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

package sources

import (
	"context"
	"testing"
	"time"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// recordingStore counts what reaches the delegate.
type recordingStore struct {
	reads   int
	writes  int
	touches int
	value   map[string]interface{}
}

func (s *recordingStore) Read(_ context.Context, _ string, _ time.Duration) (map[string]interface{}, bool, bool, time.Time, error) {
	s.reads++
	return s.value, s.value != nil, false, time.Time{}, nil
}

func (s *recordingStore) Write(_ context.Context, _, _ string, _ map[string]interface{}, _ velaprocess.SourceCacheWriteMeta) error {
	s.writes++
	return nil
}

func (s *recordingStore) Touch(_ context.Context, _ string) error {
	s.touches++
	return nil
}

// The admission dry-run must see the cache but never change it.
//
// Regression test: admission resolved sources and wrote entries through the
// Secret store, labelling them with the source type, while a real reconcile
// writes through the Config API store and labels them with the ConfigTemplate
// name. Whichever ran first won, because ApplySourceCacheMetadata is additive -
// so entries began appearing with the label the config factory does not expect.
func TestReadOnlySourceCacheStore(t *testing.T) {
	ctx := context.Background()
	inner := &recordingStore{value: map[string]interface{}{"region": "eu-west"}}
	store := NewReadOnlySourceCacheStore(inner)

	// Reads pass through: not reading would make every admission repeat the
	// source's live I/O, and would have validation resolve different data than
	// the render that follows it.
	got, found, _, _, err := store.Read(ctx, "k", time.Minute)
	if err != nil || !found || got["region"] != "eu-west" {
		t.Fatalf("reads must pass through: got=%v found=%v err=%v", got, found, err)
	}
	if inner.reads != 1 {
		t.Fatalf("expected one delegated read, got %d", inner.reads)
	}

	if err := store.Write(ctx, "k", "some-source", map[string]interface{}{"region": "x"}, velaprocess.SourceCacheWriteMeta{}); err != nil {
		t.Fatalf("a dropped write must not error: %v", err)
	}
	// Touch is optional (SourceCacheToucher), and the wrapper must still
	// implement it - otherwise a stale-served entry would fall through to the
	// delegate's Touch and advance last-accessed from a validation.
	toucher, ok := store.(velaprocess.SourceCacheToucher)
	if !ok {
		t.Fatal("the wrapper must implement SourceCacheToucher, or touches reach the delegate")
	}
	if err := toucher.Touch(ctx, "k"); err != nil {
		t.Fatalf("a dropped touch must not error: %v", err)
	}
	if inner.writes != 0 {
		t.Errorf("a validation render must not write to the shared cache; delegate saw %d writes", inner.writes)
	}
	// last-accessed drives the GC sweep, and a validation is not a use.
	if inner.touches != 0 {
		t.Errorf("a validation must not touch the cache; delegate saw %d touches", inner.touches)
	}

	// Matches the other constructors, so a nil delegate cannot produce a store
	// that looks usable.
	if NewReadOnlySourceCacheStore(nil) != nil {
		t.Error("wrapping nil must yield nil")
	}
}
