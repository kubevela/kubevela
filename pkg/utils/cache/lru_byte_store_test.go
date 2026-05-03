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

package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLRUByteStorePutGetAndCopy(t *testing.T) {
	store, err := NewLRUByteStore(10)
	require.NoError(t, err)
	defer store.Close()

	input := []byte("data")
	store.Put("key", input, 0)
	input[0] = 'D'

	got, ok, reason := store.Get("key")
	require.True(t, ok)
	require.Equal(t, MissReasonNone, reason)
	require.Equal(t, []byte("data"), got)

	got[0] = 'D'
	got, ok, reason = store.Get("key")
	require.True(t, ok)
	require.Equal(t, MissReasonNone, reason)
	require.Equal(t, []byte("data"), got)
	require.EqualValues(t, 4, store.Bytes())
}

func TestLRUByteStoreEvictsLeastRecentlyUsedToStayWithinBudget(t *testing.T) {
	store, err := NewLRUByteStore(8)
	require.NoError(t, err)
	defer store.Close()

	store.Put("first", []byte("1111"), 0)
	store.Put("second", []byte("2222"), 0)

	_, ok, reason := store.Get("first")
	require.True(t, ok)
	require.Equal(t, MissReasonNone, reason)

	store.Put("third", []byte("3333"), 0)

	_, ok, reason = store.Get("second")
	require.False(t, ok)
	require.Equal(t, MissReasonNotFound, reason)

	got, ok, reason := store.Get("first")
	require.True(t, ok)
	require.Equal(t, MissReasonNone, reason)
	require.Equal(t, []byte("1111"), got)

	got, ok, reason = store.Get("third")
	require.True(t, ok)
	require.Equal(t, MissReasonNone, reason)
	require.Equal(t, []byte("3333"), got)
	require.EqualValues(t, 8, store.Bytes())
}

func TestLRUByteStoreRejectsOversizedEntries(t *testing.T) {
	store, err := NewLRUByteStore(4)
	require.NoError(t, err)
	defer store.Close()

	store.Put("key", []byte("data"), 0)
	store.Put("key", []byte("too large"), 0)

	_, ok, reason := store.Get("key")
	require.False(t, ok)
	require.Equal(t, MissReasonNotFound, reason)
	require.Zero(t, store.Bytes())
}

func TestLRUByteStoreExpiresEntries(t *testing.T) {
	store, err := NewLRUByteStore(10)
	require.NoError(t, err)
	defer store.Close()

	store.Put("key", []byte("data"), time.Nanosecond)
	require.Eventually(t, func() bool {
		_, ok, reason := store.Get("key")
		return !ok && reason == MissReasonExpired
	}, time.Second, time.Millisecond)
	require.Zero(t, store.Bytes())
}

func TestLRUByteStoreDeleteAndClose(t *testing.T) {
	store, err := NewLRUByteStore(10)
	require.NoError(t, err)

	store.Put("key", []byte("data"), 0)
	store.Delete("key")
	_, ok, reason := store.Get("key")
	require.False(t, ok)
	require.Equal(t, MissReasonNotFound, reason)
	require.Zero(t, store.Bytes())

	store.Put("key", []byte("data"), 0)
	store.Close()
	require.Zero(t, store.Bytes())

	store.Put("key", []byte("data"), 0)
	_, ok, reason = store.Get("key")
	require.False(t, ok)
	require.Equal(t, MissReasonNotFound, reason)
}
