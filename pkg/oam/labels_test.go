package oam

import (
	"testing"
)

func TestFilterInternalMetadata(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: nil,
		},
		{
			name:     "all internal",
			input:    map[string]string{"app.oam.dev/name": "test"},
			expected: nil,
		},
		{
			name:     "external only",
			input:    map[string]string{"mycompany.io/team": "platform"},
			expected: map[string]string{"mycompany.io/team": "platform"},
		},
		{
			name:  "mixed",
			input: map[string]string{"app.oam.dev/name": "test", "mycompany.io/team": "platform"},
			expected: map[string]string{"mycompany.io/team": "platform"},
		},
		{
			name:     "helm filtered",
			input:    map[string]string{"helm.sh/chart": "mychart"},
			expected: nil,
		},
		{
			name:     "kubernetes filtered",
			input:    map[string]string{"kubernetes.io/arch": "amd64"},
			expected: nil,
		},
		{
			name:     "no slash key kept",
			input:    map[string]string{"simplekey": "value"},
			expected: map[string]string{"simplekey": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterInternalMetadata(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("key %s: expected %s, got %s", k, v, result[k])
				}
			}
		})
	}
}
