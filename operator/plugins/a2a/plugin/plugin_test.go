package plugin

import (
	"encoding/json"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
)

func TestExtractArtifactJSON(t *testing.T) {
	tests := []struct {
		name      string
		task      *a2a.Task
		wantEmpty bool // if true, expect "{}"
	}{
		{
			name: "data artifact",
			task: &a2a.Task{
				ID: "task-1",
				Status: a2a.TaskStatus{
					State: a2a.TaskStateCompleted,
				},
				Artifacts: []*a2a.Artifact{
					{
						ID: "art-1",
						Parts: a2a.ContentParts{
							a2a.DataPart{Data: map[string]any{"summary": "hello"}},
						},
					},
				},
			},
		},
		{
			name: "text artifact",
			task: &a2a.Task{
				ID: "task-2",
				Status: a2a.TaskStatus{
					State: a2a.TaskStateCompleted,
				},
				Artifacts: []*a2a.Artifact{
					{
						ID: "art-2",
						Parts: a2a.ContentParts{
							a2a.TextPart{Text: "hello world"},
						},
					},
				},
			},
		},
		{
			name: "no artifacts",
			task: &a2a.Task{
				ID: "task-3",
				Status: a2a.TaskStatus{
					State: a2a.TaskStateCompleted,
				},
			},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractArtifactJSON(tt.task)
			if tt.wantEmpty {
				if got != "{}" {
					t.Errorf("expected empty JSON, got %q", got)
				}
				return
			}
			// Should be valid JSON
			var parsed interface{}
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Errorf("expected valid JSON, got error: %v (json=%q)", err, got)
			}
			if got == "{}" {
				t.Error("expected non-empty artifact JSON")
			}
		})
	}
}

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
