package resourcekeeper

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatchContext(t *testing.T) {
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
			require.New(t).Equal(tc.cfg, *NewDispatchContext().WithOption(tc.option).GetConfig())
		})
	}
}
func TestDeleteContext(t *testing.T) {
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
			require.New(t).Equal(tc.cfg, *NewDeleteContext().WithOption(tc.option).GetConfig())
		})
	}
}

func TestGCContext(t *testing.T) {
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
			require.New(t).Equal(tc.cfg, *NewGCContext().WithOption(tc.option).GetConfig())
		})
	}
}
