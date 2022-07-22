package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterComponents(t *testing.T) {
	testCases := map[string]struct {
		Components []string
		Selector   []string
		Output     []string
	}{
		"normal": {
			Components: []string{"comp1", "comp2", "comp3"},
			Selector:   []string{"comp1", "comp2"},
			Output:     []string{"comp1", "comp2"},
		},
		"selector-empty": {
			Components: []string{"comp1", "comp2", "comp3"},
			Selector:   []string{},
			Output:     nil,
		},
		"selector-nil": {
			Components: []string{"comp1", "comp2", "comp3"},
			Selector:   nil,
			Output:     []string{"comp1", "comp2", "comp3"},
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			output := FilterComponents(tt.Components, tt.Selector)
			r.Equal(tt.Output, output)
		})
	}

}
