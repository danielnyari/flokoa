package plugin

import (
	"encoding/json"
	"time"
)

// DefaultTimeout is the default timeout for Document AI batch operations if not specified.
const DefaultTimeout = 10 * time.Minute

// DefaultPollInterval is the default interval between LRO status polls.
const DefaultPollInterval = 10 * time.Second

// MaxPollErrors is the maximum number of consecutive transient poll errors
// before permanently failing the task.
const MaxPollErrors = 5

// GCPDocAISpec defines the configuration for a GCP Document AI plugin step.
// Parsed from the Argo template plugin JSON.
type GCPDocAISpec struct {
	// ProcessorName is the full resource name of the Document AI processor.
	ProcessorName string `json:"processorName"`

	// Location is the GCP region for the regional endpoint (e.g., "us", "eu").
	Location string `json:"location"`

	// InputDocuments configures GCS input.
	InputDocuments InputDocumentsConfig `json:"inputDocuments"`

	// OutputConfig configures GCS output.
	OutputConfig OutputConfig `json:"outputConfig"`

	// ProcessOptions contains processing options.
	ProcessOptions *ProcessOptions `json:"processOptions,omitempty"`

	// SkipHumanReview skips the human review step.
	SkipHumanReview bool `json:"skipHumanReview,omitempty"`

	// Timeout is the maximum time to wait for the LRO to complete (as a duration string).
	Timeout string `json:"timeout,omitempty"`

	// Traceparent is the W3C traceparent header value injected by the compiler.
	Traceparent string `json:"traceparent,omitempty"`
}

// GetTimeout returns the timeout duration, defaulting to DefaultTimeout if not set.
func (s *GCPDocAISpec) GetTimeout() time.Duration {
	if s.Timeout != "" {
		if d, err := time.ParseDuration(s.Timeout); err == nil {
			return d
		}
	}
	return DefaultTimeout
}

// InputDocumentsConfig mirrors the GCS input configuration.
type InputDocumentsConfig struct {
	// GCSDocuments is a list of individual GCS documents.
	GCSDocuments []GCSDocument `json:"gcsDocuments,omitempty"`

	// GCSPrefix is a GCS prefix for batch input.
	GCSPrefix *GCSPrefix `json:"gcsPrefix,omitempty"`
}

// GCSDocument specifies an individual GCS document.
type GCSDocument struct {
	GCSUri   string `json:"gcsUri"`
	MimeType string `json:"mimeType"`
}

// GCSPrefix specifies a GCS prefix for batch input.
type GCSPrefix struct {
	GCSUriPrefix string `json:"gcsUriPrefix"`
}

// OutputConfig specifies the output destination.
type OutputConfig struct {
	GCSUri    string `json:"gcsUri"`
	FieldMask string `json:"fieldMask,omitempty"`
}

// ProcessOptions contains Document AI processing options.
type ProcessOptions struct {
	LayoutConfig *LayoutConfig `json:"layoutConfig,omitempty"`
}

// LayoutConfig configures layout analysis.
type LayoutConfig struct {
	ChunkingConfig *ChunkingConfig `json:"chunkingConfig,omitempty"`
}

// ChunkingConfig configures document chunking.
type ChunkingConfig struct {
	ChunkSize               int32 `json:"chunkSize,omitempty"`
	IncludeAncestorHeadings bool  `json:"includeAncestorHeadings,omitempty"`
}

// ProgressState tracks the state of an in-progress Document AI LRO between requeue calls.
type ProgressState struct {
	// OperationName is the GCP LRO operation name.
	OperationName string `json:"operationName"`

	// Location is the GCP region used for the regional endpoint.
	Location string `json:"location"`

	// StartTime is when the operation was first submitted.
	StartTime time.Time `json:"startTime"`

	// Timeout is the configured timeout duration.
	Timeout time.Duration `json:"timeout"`

	// PollErrors tracks consecutive poll errors for transient failure resilience.
	PollErrors int `json:"pollErrors,omitempty"`
}

// MarshalProgress serializes the progress state to a string for storage.
func MarshalProgress(state *ProgressState) string {
	data, _ := json.Marshal(state)
	return string(data)
}

// UnmarshalProgress deserializes the progress state from a stored string.
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

// IsTimedOut returns true if the operation has exceeded its timeout.
func (s *ProgressState) IsTimedOut() bool {
	return time.Since(s.StartTime) > s.Timeout
}
