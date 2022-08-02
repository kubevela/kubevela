/*
Copyright 2021 The KubeVela Authors.

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

package resourcekeeper

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatchOptions(t *testing.T) {
	testCases := map[string]struct {
		option DispatchOption
		cfg    dispatchConfig
	}{
		"meta-only": {
			option: MetaOnlyOption{},
			cfg:    dispatchConfig{metaOnly: true},
		},
		"skip-gc": {
			option: SkipGCOption{},
			cfg:    dispatchConfig{rtConfig: rtConfig{skipGC: true}},
		},
		"use-root": {
			option: UseRootOption{},
			cfg:    dispatchConfig{rtConfig: rtConfig{useRoot: true}},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.New(t).Equal(tc.cfg, *newDispatchConfig(tc.option))
		})
	}
}

func TestDeleteOptions(t *testing.T) {
	testCases := map[string]struct {
		option DeleteOption
		cfg    deleteConfig
	}{
		"skip-gc": {
			option: SkipGCOption{},
			cfg:    deleteConfig{rtConfig: rtConfig{skipGC: true}},
		},
		"use-root": {
			option: UseRootOption{},
			cfg:    deleteConfig{rtConfig: rtConfig{useRoot: true}},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.New(t).Equal(tc.cfg, *newDeleteConfig(tc.option))
		})
	}
}

func TestGCOptions(t *testing.T) {
	testCases := map[string]struct {
		option GCOption
		cfg    gcConfig
	}{
		"passive": {
			option: PassiveGCOption{},
			cfg:    gcConfig{passive: true},
		},
		"disable-mark-stage": {
			option: DisableMarkStageGCOption{},
			cfg:    gcConfig{disableMark: true},
		},
		"disable-gc-comp-rev": {
			option: DisableGCComponentRevisionOption{},
			cfg:    gcConfig{disableComponentRevisionGC: true},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.New(t).Equal(tc.cfg, *newGCConfig(tc.option))
		})
	}
}
