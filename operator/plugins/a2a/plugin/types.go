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

// A2ASpec defines the configuration for an A2A plugin step.
// The message field uses structured A2A message format aligned with the a2a-go library.
type A2ASpec struct {
	// Agent is the name of the Agent CR to call
	Agent string `json:"agent"`

	// Namespace is the namespace of the Agent CR (optional, defaults to workflow namespace)
	Namespace string `json:"namespace,omitempty"`

	// Message is the structured A2A message to send to the agent
	Message A2AMessage `json:"message"`

	// Config configures how the message is sent
	Config *A2ASendConfig `json:"config,omitempty"`

	// Timeout is the maximum time to wait for the task to complete (optional, default 5m)
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Traceparent is the W3C traceparent header value injected by the compiler
	// from the controller's span context. The plugin uses it to restore the
	// distributed trace and propagate it to the downstream agent.
	Traceparent string `json:"traceparent,omitempty"`
}

// A2AMessage represents an A2A protocol message in the plugin spec.
// Aligns with a2a.Message from github.com/a2aproject/a2a-go.
type A2AMessage struct {
	// Role of the message sender (default: "user")
	Role string `json:"role,omitempty"`

	// Parts is the content of the message
	Parts []A2AMessagePart `json:"parts"`

	// ContextID for conversation threading
	ContextID string `json:"contextId,omitempty"`

	// ReferenceTaskIDs links to previous A2A task IDs
	ReferenceTaskIDs []string `json:"referenceTaskIds,omitempty"`

	// Extensions lists A2A extension URIs to activate
	Extensions []string `json:"extensions,omitempty"`

	// TaskID continues an existing A2A task instead of creating a new one
	TaskID string `json:"taskId,omitempty"`

	// Metadata is arbitrary key-value metadata
	Metadata map[string]any `json:"metadata,omitempty"`
}

// A2AMessagePart represents a single content part. Exactly one field should be set.
// Aligns with a2a.Part (TextPart, DataPart, FilePart) from github.com/a2aproject/a2a-go.
type A2AMessagePart struct {
	Text *A2ATextPart `json:"text,omitempty"`
	Data *A2ADataPart `json:"data,omitempty"`
	File *A2AFilePart `json:"file,omitempty"`
}

// A2ATextPart contains text content. Aligns with a2a.TextPart.
type A2ATextPart struct {
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// A2ADataPart contains structured JSON data. Aligns with a2a.DataPart.
type A2ADataPart struct {
	Data     map[string]any `json:"data"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// A2AFilePart contains file content. Aligns with a2a.FilePart.
type A2AFilePart struct {
	File     A2AFileContent `json:"file"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// A2AFileContent represents file data. Aligns with a2a.FileBytes / a2a.FileURI.
type A2AFileContent struct {
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Bytes    string `json:"bytes,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// A2ASendConfig configures message sending. Aligns with a2a.MessageSendConfig.
type A2ASendConfig struct {
	AcceptedOutputModes    []string                    `json:"acceptedOutputModes,omitempty"`
	Blocking               *bool                       `json:"blocking,omitempty"`
	HistoryLength          *int                        `json:"historyLength,omitempty"`
	PushNotificationConfig *A2APushNotificationConfig  `json:"pushNotificationConfig,omitempty"`
}

// A2APushNotificationConfig configures push notifications. Aligns with a2a.PushNotificationConfig.
type A2APushNotificationConfig struct {
	URL            string              `json:"url"`
	ID             string              `json:"id,omitempty"`
	Token          string              `json:"token,omitempty"`
	Authentication *A2APushNotificationAuth `json:"authentication,omitempty"`
}

// A2APushNotificationAuth configures push notification authentication. Aligns with a2a.PushNotificationAuthInfo.
type A2APushNotificationAuth struct {
	Schemes     []string `json:"schemes"`
	Credentials string   `json:"credentials,omitempty"`
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
