package converter

import (
	"encoding/json"
	"reflect"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

const testModelName = "gpt-5-mini"

func TestModelToProto_Nil(t *testing.T) {
	result := ModelToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelToProto(t *testing.T) {
	maxTokens := int32(4096)
	model := &agentv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testModelName,
			Namespace: "models",
		},
		Spec: agentv1alpha1.ModelSpec{
			Model: testModelName,
			ProviderRef: agentv1alpha1.ProviderRef{
				Name:      "openai-provider",
				Namespace: "providers",
			},
			Settings: &agentv1alpha1.ModelSettings{
				Temperature: "0.7",
				MaxTokens:   &maxTokens,
			},
		},
		Status: agentv1alpha1.ModelStatus{
			Ready:              true,
			ObservedGeneration: 3,
			ResolvedProvider: &agentv1alpha1.ResolvedProviderInfo{
				Provider:  agentv1alpha1.ProviderTypeOpenAI,
				Name:      "openai-provider",
				Namespace: "providers",
			},
		},
	}

	result := ModelToProto(model)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil || result.Metadata.Name != testModelName {
		t.Fatal("expected metadata with name " + testModelName)
	}
	if result.Spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if result.Status == nil {
		t.Fatal("expected non-nil status")
	}
}

func TestModelSpecToProto_Nil(t *testing.T) {
	result := ModelSpecToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelSpecToProto(t *testing.T) {
	maxTokens := int32(4096)
	spec := &agentv1alpha1.ModelSpec{
		Model: testModelName,
		ProviderRef: agentv1alpha1.ProviderRef{
			Name:      "openai-provider",
			Namespace: "providers",
		},
		Settings: &agentv1alpha1.ModelSettings{
			Temperature: "0.7",
			MaxTokens:   &maxTokens,
		},
	}

	result := ModelSpecToProto(spec)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Model != testModelName {
		t.Fatalf("expected model %s, got %q", testModelName, result.Model)
	}
	if result.ProviderRef == nil {
		t.Fatal("expected provider ref to be set")
	}
	if result.ProviderRef.Name != "openai-provider" {
		t.Fatalf("expected openai-provider, got %q", result.ProviderRef.Name)
	}
	if result.ProviderRef.Namespace != "providers" {
		t.Fatalf("expected providers namespace, got %q", result.ProviderRef.Namespace)
	}
	if result.Settings == nil {
		t.Fatal("expected settings to be set")
	}
	if result.Settings.Temperature != "0.7" {
		t.Fatalf("expected temperature 0.7, got %q", result.Settings.Temperature)
	}
}

func TestModelSpecToProto_NilSettings(t *testing.T) {
	spec := &agentv1alpha1.ModelSpec{
		Model: testModelName,
		ProviderRef: agentv1alpha1.ProviderRef{
			Name: "provider",
		},
	}

	result := ModelSpecToProto(spec)
	if result.Settings != nil {
		t.Fatal("expected nil settings")
	}
}

func TestModelSettingsToProto_Nil(t *testing.T) {
	result := ModelSettingsToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelSettingsToProto_AllFields(t *testing.T) {
	maxTokens := int32(4096)
	topK := int32(50)
	timeout := int32(60)
	parallelToolCalls := true
	seed := int64(12345)

	settings := &agentv1alpha1.ModelSettings{
		Temperature:       "0.8",
		MaxTokens:         &maxTokens,
		TopP:              "0.95",
		TopK:              &topK,
		PresencePenalty:   "0.5",
		FrequencyPenalty:  "0.3",
		TimeoutSeconds:    &timeout,
		ParallelToolCalls: &parallelToolCalls,
		Seed:              &seed,
		StopSequences:     []string{"END", "STOP"},
		ExtraHeaders:      map[string]string{"X-Custom": "value"},
		LogitBias:         map[string]int32{"100": 1, "200": -1},
		Extra:             &apiextensionsv1.JSON{Raw: []byte(`{"service_tier":"flex","thinking":{"budget":1024}}`)},
	}

	result := ModelSettingsToProto(settings)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Temperature != "0.8" {
		t.Fatalf("expected temperature 0.8, got %q", result.Temperature)
	}
	if result.MaxTokens != 4096 {
		t.Fatalf("expected max tokens 4096, got %d", result.MaxTokens)
	}
	if result.TopP != "0.95" {
		t.Fatalf("expected top_p 0.95, got %q", result.TopP)
	}
	if result.TopK != 50 {
		t.Fatalf("expected top_k 50, got %d", result.TopK)
	}
	if result.PresencePenalty != "0.5" {
		t.Fatalf("expected presence penalty 0.5, got %q", result.PresencePenalty)
	}
	if result.FrequencyPenalty != "0.3" {
		t.Fatalf("expected frequency penalty 0.3, got %q", result.FrequencyPenalty)
	}
	if result.TimeoutSeconds != 60 {
		t.Fatalf("expected timeout 60, got %d", result.TimeoutSeconds)
	}
	if !result.ParallelToolCalls {
		t.Fatal("expected parallel tool calls true")
	}
	if result.Seed != 12345 {
		t.Fatalf("expected seed 12345, got %d", result.Seed)
	}
	if len(result.StopSequences) != 2 {
		t.Fatalf("expected 2 stop sequences, got %d", len(result.StopSequences))
	}
	if result.ExtraHeaders["X-Custom"] != "value" {
		t.Fatal("expected extra headers")
	}
	if result.LogitBias["100"] != 1 || result.LogitBias["200"] != -1 {
		t.Fatal("expected logit bias")
	}
	if result.Extra == nil {
		t.Fatal("expected extra to be set")
	}
	if got := result.Extra.Fields["service_tier"].GetStringValue(); got != "flex" {
		t.Fatalf("expected service_tier flex, got %q", got)
	}
	thinking := result.Extra.Fields["thinking"].GetStructValue()
	if thinking == nil || thinking.Fields["budget"].GetNumberValue() != 1024 {
		t.Fatal("expected nested thinking.budget 1024")
	}
}

func TestModelSettingsToProto_NilOptionalFields(t *testing.T) {
	settings := &agentv1alpha1.ModelSettings{
		Temperature: "0.5",
	}

	result := ModelSettingsToProto(settings)
	if result.MaxTokens != 0 {
		t.Fatalf("expected 0 max tokens for nil, got %d", result.MaxTokens)
	}
	if result.TopK != 0 {
		t.Fatalf("expected 0 top_k for nil, got %d", result.TopK)
	}
	if result.TimeoutSeconds != 0 {
		t.Fatalf("expected 0 timeout for nil, got %d", result.TimeoutSeconds)
	}
	if result.ParallelToolCalls {
		t.Fatal("expected false parallel tool calls for nil")
	}
	if result.Seed != 0 {
		t.Fatalf("expected 0 seed for nil, got %d", result.Seed)
	}
	if result.Extra != nil {
		t.Fatal("expected nil extra for nil input")
	}
}

func TestModelStatusToProto_Nil(t *testing.T) {
	result := ModelStatusToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelStatusToProto(t *testing.T) {
	status := &agentv1alpha1.ModelStatus{
		ObservedGeneration: 5,
		Ready:              true,
		ResolvedProvider: &agentv1alpha1.ResolvedProviderInfo{
			Provider:  agentv1alpha1.ProviderTypeAnthropic,
			Name:      "anthropic-provider",
			Namespace: "ns",
		},
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
		},
	}

	result := ModelStatusToProto(status)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ObservedGeneration != 5 {
		t.Fatalf("expected observed gen 5, got %d", result.ObservedGeneration)
	}
	if !result.Ready {
		t.Fatal("expected ready true")
	}
	if result.ResolvedProvider == nil {
		t.Fatal("expected resolved provider")
	}
	if result.ResolvedProvider.Provider != pb.ProviderType_PROVIDER_TYPE_ANTHROPIC {
		t.Fatalf("expected anthropic, got %v", result.ResolvedProvider.Provider)
	}
	if result.ResolvedProvider.Name != "anthropic-provider" {
		t.Fatalf("expected anthropic-provider, got %q", result.ResolvedProvider.Name)
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(result.Conditions))
	}
}

func TestModelStatusToProto_NilResolvedProvider(t *testing.T) {
	status := &agentv1alpha1.ModelStatus{
		Ready: false,
	}

	result := ModelStatusToProto(status)
	if result.ResolvedProvider != nil {
		t.Fatal("expected nil resolved provider")
	}
}

func TestModelListToProto_Nil(t *testing.T) {
	result := ModelListToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelListToProto(t *testing.T) {
	list := &agentv1alpha1.ModelList{
		ListMeta: metav1.ListMeta{ResourceVersion: "50"},
		Items: []agentv1alpha1.Model{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "model-1"},
				Spec:       agentv1alpha1.ModelSpec{Model: testModelName},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "model-2"},
				Spec:       agentv1alpha1.ModelSpec{Model: "claude-sonnet-4-5"},
			},
		},
	}

	result := ModelListToProto(list)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil || result.Metadata.ResourceVersion != "50" {
		t.Fatal("expected metadata")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
}

func TestModelListToProto_Empty(t *testing.T) {
	list := &agentv1alpha1.ModelList{
		Items: []agentv1alpha1.Model{},
	}

	result := ModelListToProto(list)
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
}

func TestModelFromProto_Nil(t *testing.T) {
	result := ModelFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelFromProto(t *testing.T) {
	proto := &pb.Model{
		Metadata: &pb.ObjectMeta{
			Name:      testModelName,
			Namespace: "models",
		},
		Spec: &pb.ModelSpec{
			Model: testModelName,
			ProviderRef: &pb.ProviderRef{
				Name:      "openai",
				Namespace: "providers",
			},
			Settings: &pb.ModelSettings{
				Temperature: "0.7",
				MaxTokens:   4096,
			},
		},
	}

	result := ModelFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != testModelName {
		t.Fatalf("expected name %s, got %q", testModelName, result.Name)
	}
	if result.Spec.Model != testModelName {
		t.Fatalf("expected model %s, got %q", testModelName, result.Spec.Model)
	}
	if result.Spec.ProviderRef.Name != "openai" {
		t.Fatalf("expected provider openai, got %q", result.Spec.ProviderRef.Name)
	}
	if result.Spec.Settings == nil {
		t.Fatal("expected settings to be set")
	}
	if result.Spec.Settings.MaxTokens == nil || *result.Spec.Settings.MaxTokens != 4096 {
		t.Fatal("expected max tokens 4096")
	}
}

func TestModelFromProto_NilFields(t *testing.T) {
	proto := &pb.Model{}

	result := ModelFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "" {
		t.Fatalf("expected empty name, got %q", result.Name)
	}
}

func TestModelSpecFromProto_Nil(t *testing.T) {
	result := ModelSpecFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelSpecFromProto(t *testing.T) {
	proto := &pb.ModelSpec{
		Model: "claude-sonnet-4-5",
		ProviderRef: &pb.ProviderRef{
			Name:      "anthropic",
			Namespace: "ns",
		},
		Settings: &pb.ModelSettings{
			Temperature: "0.5",
			MaxTokens:   2048,
			TopK:        40,
		},
	}

	result := ModelSpecFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected model, got %q", result.Model)
	}
	if result.ProviderRef.Name != "anthropic" {
		t.Fatalf("expected provider anthropic, got %q", result.ProviderRef.Name)
	}
	if result.Settings == nil {
		t.Fatal("expected settings")
	}
	if result.Settings.Temperature != "0.5" {
		t.Fatalf("expected temperature 0.5, got %q", result.Settings.Temperature)
	}
	if result.Settings.TopK == nil || *result.Settings.TopK != 40 {
		t.Fatal("expected top_k 40")
	}
}

func TestModelSpecFromProto_NilProviderRef(t *testing.T) {
	proto := &pb.ModelSpec{
		Model: "test",
	}

	result := ModelSpecFromProto(proto)
	if result.ProviderRef.Name != "" {
		t.Fatal("expected empty provider ref")
	}
	if result.Settings != nil {
		t.Fatal("expected nil settings")
	}
}

func TestModelSettingsFromProto_Nil(t *testing.T) {
	result := ModelSettingsFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelSettingsFromProto(t *testing.T) {
	proto := &pb.ModelSettings{
		Temperature:       "0.8",
		MaxTokens:         4096,
		TopP:              "0.95",
		TopK:              50,
		PresencePenalty:   "0.5",
		FrequencyPenalty:  "0.3",
		TimeoutSeconds:    60,
		ParallelToolCalls: true,
		Seed:              12345,
		StopSequences:     []string{"END"},
		ExtraHeaders:      map[string]string{"X-H": "v"},
		LogitBias:         map[string]int32{"50": 1},
	}

	result := ModelSettingsFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Temperature != "0.8" {
		t.Fatalf("expected temperature 0.8, got %q", result.Temperature)
	}
	if result.MaxTokens == nil || *result.MaxTokens != 4096 {
		t.Fatal("expected max tokens 4096")
	}
	if result.TopK == nil || *result.TopK != 50 {
		t.Fatal("expected top_k 50")
	}
	if result.TimeoutSeconds == nil || *result.TimeoutSeconds != 60 {
		t.Fatal("expected timeout 60")
	}
	if result.ParallelToolCalls == nil || !*result.ParallelToolCalls {
		t.Fatal("expected parallel tool calls true")
	}
	if result.Seed == nil || *result.Seed != 12345 {
		t.Fatal("expected seed 12345")
	}
	if len(result.StopSequences) != 1 {
		t.Fatalf("expected 1 stop sequence, got %d", len(result.StopSequences))
	}
	if result.ExtraHeaders["X-H"] != "v" {
		t.Fatal("expected extra headers")
	}
	if result.LogitBias["50"] != 1 {
		t.Fatal("expected logit bias")
	}
	if result.Extra != nil {
		t.Fatal("expected nil extra when proto extra unset")
	}
}

func TestModelSettingsFromProto_ZeroOptionalFields(t *testing.T) {
	proto := &pb.ModelSettings{
		Temperature: "0.5",
	}

	result := ModelSettingsFromProto(proto)
	if result.MaxTokens != nil {
		t.Fatal("expected nil max tokens for 0")
	}
	if result.TopK != nil {
		t.Fatal("expected nil top_k for 0")
	}
	if result.TimeoutSeconds != nil {
		t.Fatal("expected nil timeout for 0")
	}
	if result.ParallelToolCalls != nil {
		t.Fatal("expected nil parallel tool calls for false")
	}
	if result.Seed != nil {
		t.Fatal("expected nil seed for 0")
	}
}

func TestModelSettingsRoundTrip(t *testing.T) {
	maxTokens := int32(2048)
	topK := int32(40)
	timeout := int32(30)
	parallelToolCalls := true
	seed := int64(42)

	original := &agentv1alpha1.ModelSettings{
		Temperature:       "0.7",
		MaxTokens:         &maxTokens,
		TopP:              "0.9",
		TopK:              &topK,
		PresencePenalty:   "0.1",
		FrequencyPenalty:  "0.2",
		TimeoutSeconds:    &timeout,
		ParallelToolCalls: &parallelToolCalls,
		Seed:              &seed,
		StopSequences:     []string{"STOP"},
		ExtraHeaders:      map[string]string{"H": "V"},
		LogitBias:         map[string]int32{"10": 2},
		Extra:             &apiextensionsv1.JSON{Raw: []byte(`{"service_tier":"flex","extra_body":{"top_a":0.5}}`)},
	}

	proto := ModelSettingsToProto(original)
	roundTrip := ModelSettingsFromProto(proto)

	if roundTrip.Temperature != original.Temperature {
		t.Fatalf("temperature mismatch: %q vs %q", roundTrip.Temperature, original.Temperature)
	}
	if *roundTrip.MaxTokens != *original.MaxTokens {
		t.Fatal("max tokens mismatch")
	}
	if roundTrip.TopP != original.TopP {
		t.Fatal("top_p mismatch")
	}
	if *roundTrip.TopK != *original.TopK {
		t.Fatal("top_k mismatch")
	}
	if roundTrip.PresencePenalty != original.PresencePenalty {
		t.Fatal("presence penalty mismatch")
	}
	if roundTrip.FrequencyPenalty != original.FrequencyPenalty {
		t.Fatal("frequency penalty mismatch")
	}
	if *roundTrip.TimeoutSeconds != *original.TimeoutSeconds {
		t.Fatal("timeout mismatch")
	}
	if *roundTrip.ParallelToolCalls != *original.ParallelToolCalls {
		t.Fatal("parallel tool calls mismatch")
	}
	if *roundTrip.Seed != *original.Seed {
		t.Fatal("seed mismatch")
	}
	if len(roundTrip.StopSequences) != 1 || roundTrip.StopSequences[0] != "STOP" {
		t.Fatal("stop sequences mismatch")
	}
	if roundTrip.ExtraHeaders["H"] != "V" {
		t.Fatal("extra headers mismatch")
	}
	if roundTrip.LogitBias["10"] != 2 {
		t.Fatal("logit bias mismatch")
	}
	if roundTrip.Extra == nil {
		t.Fatal("expected extra to round-trip")
	}
	if !jsonSemanticallyEqual(t, original.Extra.Raw, roundTrip.Extra.Raw) {
		t.Fatalf("expected extra JSON to be semantically equal: %s vs %s", original.Extra.Raw, roundTrip.Extra.Raw)
	}
}

func TestProviderTypeToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.ProviderType
		expected pb.ProviderType
	}{
		{agentv1alpha1.ProviderTypeOpenAI, pb.ProviderType_PROVIDER_TYPE_OPENAI},
		{agentv1alpha1.ProviderTypeAnthropic, pb.ProviderType_PROVIDER_TYPE_ANTHROPIC},
		{agentv1alpha1.ProviderTypeGoogle, pb.ProviderType_PROVIDER_TYPE_GOOGLE},
		{agentv1alpha1.ProviderTypeBedrock, pb.ProviderType_PROVIDER_TYPE_BEDROCK},
		{agentv1alpha1.ProviderType("unknown"), pb.ProviderType_PROVIDER_TYPE_UNSPECIFIED},
		{agentv1alpha1.ProviderType(""), pb.ProviderType_PROVIDER_TYPE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := ProviderTypeToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("ProviderTypeToProto(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestProviderTypeFromProto(t *testing.T) {
	tests := []struct {
		input    pb.ProviderType
		expected agentv1alpha1.ProviderType
	}{
		{pb.ProviderType_PROVIDER_TYPE_OPENAI, agentv1alpha1.ProviderTypeOpenAI},
		{pb.ProviderType_PROVIDER_TYPE_ANTHROPIC, agentv1alpha1.ProviderTypeAnthropic},
		{pb.ProviderType_PROVIDER_TYPE_GOOGLE, agentv1alpha1.ProviderTypeGoogle},
		{pb.ProviderType_PROVIDER_TYPE_BEDROCK, agentv1alpha1.ProviderTypeBedrock},
		{pb.ProviderType_PROVIDER_TYPE_UNSPECIFIED, agentv1alpha1.ProviderType("")},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			result := ProviderTypeFromProto(tt.input)
			if result != tt.expected {
				t.Fatalf("ProviderTypeFromProto(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestProviderTypeRoundTrip(t *testing.T) {
	types := []agentv1alpha1.ProviderType{
		agentv1alpha1.ProviderTypeOpenAI,
		agentv1alpha1.ProviderTypeAnthropic,
		agentv1alpha1.ProviderTypeGoogle,
		agentv1alpha1.ProviderTypeBedrock,
	}

	for _, pt := range types {
		t.Run(string(pt), func(t *testing.T) {
			proto := ProviderTypeToProto(pt)
			roundTrip := ProviderTypeFromProto(proto)
			if roundTrip != pt {
				t.Fatalf("round-trip failed: %q -> %v -> %q", pt, proto, roundTrip)
			}
		})
	}
}

// jsonSemanticallyEqual reports whether two raw JSON documents encode the
// same value, ignoring key order and formatting.
func jsonSemanticallyEqual(t *testing.T, a, b []byte) bool {
	t.Helper()

	var left any
	var right any
	if err := json.Unmarshal(a, &left); err != nil {
		t.Fatalf("failed to unmarshal left JSON: %v", err)
	}
	if err := json.Unmarshal(b, &right); err != nil {
		t.Fatalf("failed to unmarshal right JSON: %v", err)
	}

	return reflect.DeepEqual(left, right)
}
