package plugin

import "testing"

func TestEndpointCandidates(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "service root adds a2a fallback",
			input:    "http://petstore-agent.flokoa-system.svc.cluster.local/",
			expected: []string{"http://petstore-agent.flokoa-system.svc.cluster.local", "http://petstore-agent.flokoa-system.svc.cluster.local/a2a"},
		},
		{
			name:     "a2a path tries root fallback",
			input:    "http://petstore-agent.flokoa-system.svc.cluster.local/a2a",
			expected: []string{"http://petstore-agent.flokoa-system.svc.cluster.local/a2a", "http://petstore-agent.flokoa-system.svc.cluster.local"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := endpointCandidates(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("unexpected candidate count: got=%d want=%d candidates=%v", len(got), len(tt.expected), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Fatalf("candidate[%d] mismatch: got=%q want=%q all=%v", i, got[i], tt.expected[i], got)
				}
			}
		})
	}
}
