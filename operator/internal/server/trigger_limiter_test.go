package server

import (
	"strings"
	"testing"
	"time"

	triggerapp "github.com/danielnyari/flokoa/internal/app/trigger"
)

func int32Ptr(v int32) *int32 { return &v }

func TestTriggerLimiter_Check_NilLimits(t *testing.T) {
	l := NewTriggerLimiter()

	if err := l.Check("my-trigger", "default", nil); err != nil {
		t.Fatalf("expected nil error for nil limits, got: %v", err)
	}
}

func TestTriggerLimiter_MaxInvocationsPerHour(t *testing.T) {
	tests := []struct {
		name       string
		limit      int32
		calls      int
		wantErrIdx int // index of the first call that should fail (-1 = none)
	}{
		{
			name:       "allow up to limit",
			limit:      3,
			calls:      3,
			wantErrIdx: -1,
		},
		{
			name:       "reject when exceeded",
			limit:      2,
			calls:      3,
			wantErrIdx: 2,
		},
		{
			name:       "limit of 1 rejects second call",
			limit:      1,
			calls:      2,
			wantErrIdx: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewTriggerLimiter()
			limits := &triggerapp.LimitsConfig{
				MaxInvocationsPerHour: int32Ptr(tt.limit),
			}

			for i := range tt.calls {
				err := l.Check("t", "ns", limits)
				if tt.wantErrIdx >= 0 && i >= tt.wantErrIdx {
					if err == nil {
						t.Fatalf("call %d: expected error, got nil", i)
					}
					if !strings.Contains(err.Error(), "invocations_per_hour limit exceeded") {
						t.Fatalf("call %d: unexpected error message: %v", i, err)
					}
				} else {
					if err != nil {
						t.Fatalf("call %d: unexpected error: %v", i, err)
					}
				}
			}
		})
	}
}

func TestTriggerLimiter_MaxConcurrentTasks(t *testing.T) {
	tests := []struct {
		name       string
		limit      int32
		calls      int
		wantErrIdx int
	}{
		{
			name:       "allow up to limit",
			limit:      3,
			calls:      3,
			wantErrIdx: -1,
		},
		{
			name:       "reject when exceeded",
			limit:      2,
			calls:      3,
			wantErrIdx: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewTriggerLimiter()
			limits := &triggerapp.LimitsConfig{
				MaxConcurrentTasks: int32Ptr(tt.limit),
			}

			for i := range tt.calls {
				err := l.Check("t", "ns", limits)
				if tt.wantErrIdx >= 0 && i >= tt.wantErrIdx {
					if err == nil {
						t.Fatalf("call %d: expected error, got nil", i)
					}
					if !strings.Contains(err.Error(), "concurrent_tasks limit exceeded") {
						t.Fatalf("call %d: unexpected error message: %v", i, err)
					}
				} else {
					if err != nil {
						t.Fatalf("call %d: unexpected error: %v", i, err)
					}
				}
			}
		})
	}
}

func TestTriggerLimiter_TaskCompleted(t *testing.T) {
	l := NewTriggerLimiter()
	limits := &triggerapp.LimitsConfig{
		MaxConcurrentTasks: int32Ptr(1),
	}

	// First call succeeds
	if err := l.Check("t", "ns", limits); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}

	// Second call should fail (concurrent limit = 1, one active)
	if err := l.Check("t", "ns", limits); err == nil {
		t.Fatal("second call: expected error, got nil")
	}

	// Complete the first task
	l.TaskCompleted("t", "ns")

	// Third call should succeed again
	if err := l.Check("t", "ns", limits); err != nil {
		t.Fatalf("third call after TaskCompleted: unexpected error: %v", err)
	}
}

func TestTriggerLimiter_TaskCompleted_NeverNegative(t *testing.T) {
	l := NewTriggerLimiter()

	// Complete task for a key that has no counters yet — should not panic
	l.TaskCompleted("nonexistent", "ns")

	// Create a counter, then complete more than active
	limits := &triggerapp.LimitsConfig{
		MaxConcurrentTasks: int32Ptr(5),
	}
	if err := l.Check("t", "ns", limits); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	l.TaskCompleted("t", "ns")
	l.TaskCompleted("t", "ns") // extra decrement — should stay at 0

	l.mu.Lock()
	active := l.counters["ns/t"].activeTasks
	l.mu.Unlock()

	if active != 0 {
		t.Fatalf("expected activeTasks=0 after extra decrement, got %d", active)
	}
}

func TestTriggerLimiter_OldInvocationsPruned(t *testing.T) {
	l := NewTriggerLimiter()
	limits := &triggerapp.LimitsConfig{
		MaxInvocationsPerHour: int32Ptr(1),
	}

	// First call succeeds
	if err := l.Check("t", "ns", limits); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}

	// Second call should fail (limit = 1)
	if err := l.Check("t", "ns", limits); err == nil {
		t.Fatal("second call: expected error, got nil")
	}

	// Manually backdate the recorded invocation to >1 hour ago
	l.mu.Lock()
	c := l.counters["ns/t"]
	c.invocations[0] = time.Now().Add(-2 * time.Hour)
	l.mu.Unlock()

	// Third call should succeed because the old invocation gets pruned
	if err := l.Check("t", "ns", limits); err != nil {
		t.Fatalf("third call after pruning: unexpected error: %v", err)
	}
}

func TestTriggerLimiter_SeparateKeys(t *testing.T) {
	l := NewTriggerLimiter()
	limits := &triggerapp.LimitsConfig{
		MaxInvocationsPerHour: int32Ptr(1),
	}

	// Trigger A in namespace ns1
	if err := l.Check("a", "ns1", limits); err != nil {
		t.Fatalf("trigger a/ns1: unexpected error: %v", err)
	}

	// Trigger A in namespace ns2 — different key, should succeed
	if err := l.Check("a", "ns2", limits); err != nil {
		t.Fatalf("trigger a/ns2: unexpected error: %v", err)
	}

	// Trigger B in namespace ns1 — different key, should succeed
	if err := l.Check("b", "ns1", limits); err != nil {
		t.Fatalf("trigger b/ns1: unexpected error: %v", err)
	}
}
