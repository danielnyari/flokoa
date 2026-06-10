package server

import (
	"fmt"
	"sync"
	"time"

	triggerapp "github.com/danielnyari/flokoa/internal/app/trigger"
)

// TriggerLimiter enforces per-trigger rate and cost limits in-memory.
// Authoritative limit enforcement lives here in flokoa-server's data plane.
// These are best-effort sliding window counters, reset on process restart.
type TriggerLimiter struct {
	mu       sync.Mutex
	counters map[string]*triggerCounters
}

type triggerCounters struct {
	invocations []time.Time // timestamps of invocations in the last hour
	activeTasks int32       // currently in-flight tasks
}

// NewTriggerLimiter creates a new trigger limiter.
func NewTriggerLimiter() *TriggerLimiter {
	return &TriggerLimiter{
		counters: make(map[string]*triggerCounters),
	}
}

// Check verifies all configured limits for a trigger. Returns an error if any limit is exceeded.
func (l *TriggerLimiter) Check(name, namespace string, limits *triggerapp.LimitsConfig) error {
	if limits == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	key := namespace + "/" + name
	c, ok := l.counters[key]
	if !ok {
		c = &triggerCounters{}
		l.counters[key] = c
	}

	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	// Prune old invocations outside the 1-hour window
	pruned := c.invocations[:0]
	for _, t := range c.invocations {
		if t.After(oneHourAgo) {
			pruned = append(pruned, t)
		}
	}
	c.invocations = pruned

	// Check maxInvocationsPerHour
	if limits.MaxInvocationsPerHour != nil && *limits.MaxInvocationsPerHour > 0 {
		if int32(len(c.invocations)) >= *limits.MaxInvocationsPerHour {
			return fmt.Errorf("invocations_per_hour limit exceeded: %d/%d", len(c.invocations), *limits.MaxInvocationsPerHour)
		}
	}

	// Check maxConcurrentTasks
	if limits.MaxConcurrentTasks != nil && *limits.MaxConcurrentTasks > 0 {
		if c.activeTasks >= *limits.MaxConcurrentTasks {
			return fmt.Errorf("concurrent_tasks limit exceeded: %d/%d", c.activeTasks, *limits.MaxConcurrentTasks)
		}
	}

	// Record this invocation
	c.invocations = append(c.invocations, now)
	c.activeTasks++

	return nil
}

// TaskCompleted decrements the active task counter for a trigger.
func (l *TriggerLimiter) TaskCompleted(name, namespace string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := namespace + "/" + name
	if c, ok := l.counters[key]; ok {
		if c.activeTasks > 0 {
			c.activeTasks--
		}
	}
}
