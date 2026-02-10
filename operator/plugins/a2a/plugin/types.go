package plugin

import (
	"encoding/json"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultTimeout is the default timeout for A2A tasks if not specified
const DefaultTimeout = 5 * time.Minute

// DefaultPollInterval is the default interval between task status polls
const DefaultPollInterval = 5 * time.Second

// A2ASpec defines the configuration for an A2A plugin step
type A2ASpec struct {
	// Agent is the name of the Agent CR to call
	Agent string `json:"agent"`

	// Namespace is the namespace of the Agent CR (optional, defaults to workflow namespace)
	Namespace string `json:"namespace,omitempty"`

	// Message is the message to send to the agent
	Message string `json:"message"`

	// Timeout is the maximum time to wait for the task to complete (optional, default 5m)
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// GetTimeout returns the timeout duration, defaulting to DefaultTimeout if not set
func (s *A2ASpec) GetTimeout() time.Duration {
	if s.Timeout != nil {
		return s.Timeout.Duration
	}
	return DefaultTimeout
}

// ProgressState tracks the state of an in-progress A2A task between requeue calls
type ProgressState struct {
	// TaskID is the A2A task identifier
	TaskID string `json:"taskId"`

	// ContextID is the A2A context identifier
	ContextID string `json:"contextId,omitempty"`

	// Endpoint is the resolved agent endpoint URL
	Endpoint string `json:"endpoint"`

	// StartTime is when the task was first submitted
	StartTime time.Time `json:"startTime"`

	// Timeout is the configured timeout duration
	Timeout time.Duration `json:"timeout"`
}

// MarshalProgress serializes the progress state to a string for storage in Argo node progress
func MarshalProgress(state *ProgressState) string {
	data, _ := json.Marshal(state)
	return string(data)
}

// UnmarshalProgress deserializes the progress state from the Argo node progress string
func UnmarshalProgress(progress string) (*ProgressState, error) {
	if progress == "" {
		return nil, nil
	}
	var state ProgressState
	if err := json.Unmarshal([]byte(progress), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// IsTimedOut returns true if the task has exceeded its timeout
func (s *ProgressState) IsTimedOut() bool {
	return time.Since(s.StartTime) > s.Timeout
}
