package compiler

import (
	"bytes"
	"encoding/json"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/domain/hash"
)

// applyFragment lays the typed fragment fields over the extra layer.
// Fragment instructions are handled by applyInstructions (list semantics).
func applyFragment(doc map[string]any, frag *agentv1alpha1.AgentSpecFragment) {
	if frag.Model != "" {
		doc["model"] = frag.Model
	}
	if frag.Name != "" {
		doc["name"] = frag.Name
	}
	if frag.Description != "" {
		doc["description"] = frag.Description
	}
	if frag.OutputSchema != nil {
		var schema map[string]any
		if err := json.Unmarshal(frag.OutputSchema.Raw, &schema); err == nil {
			doc["output_schema"] = schema
		}
	}
	if frag.ModelSettings != nil {
		settings := settingsToMap(frag.ModelSettings)
		if len(settings) > 0 {
			// Merge over extra's model_settings if present (typed wins per-key).
			if existing, ok := doc["model_settings"].(map[string]any); ok {
				for k, v := range settings {
					existing[k] = v
				}
				settings = existing
			}
			doc["model_settings"] = settings
		}
	}
	for _, entry := range frag.Capabilities {
		var config map[string]any
		if entry.Config != nil {
			_ = json.Unmarshal(entry.Config.Raw, &config)
		}
		doc["capabilities"] = append(capabilityList(doc), capabilityEntry(entry.Name, config))
	}
}

// settingsToMap converts typed ModelSettings to compiled model_settings keys.
// Decimal strings become JSON numbers; Extra merges lowest-precedence.
func settingsToMap(s *agentv1alpha1.ModelSettings) map[string]any {
	out := map[string]any{}
	if s.Extra != nil {
		var extra map[string]any
		if err := json.Unmarshal(s.Extra.Raw, &extra); err == nil {
			for k, v := range extra {
				out[k] = v
			}
		}
	}
	if s.MaxTokens != nil {
		out["max_tokens"] = *s.MaxTokens
	}
	if s.Temperature != "" {
		out["temperature"] = json.Number(s.Temperature)
	}
	if s.TopP != "" {
		out["top_p"] = json.Number(s.TopP)
	}
	if s.TopK != nil {
		out["top_k"] = *s.TopK
	}
	if s.TimeoutSeconds != nil {
		out["timeout"] = *s.TimeoutSeconds
	}
	if s.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *s.ParallelToolCalls
	}
	if s.Seed != nil {
		out["seed"] = *s.Seed
	}
	if s.PresencePenalty != "" {
		out["presence_penalty"] = json.Number(s.PresencePenalty)
	}
	if s.FrequencyPenalty != "" {
		out["frequency_penalty"] = json.Number(s.FrequencyPenalty)
	}
	if len(s.LogitBias) > 0 {
		out["logit_bias"] = s.LogitBias
	}
	if len(s.StopSequences) > 0 {
		out["stop_sequences"] = s.StopSequences
	}
	if len(s.ExtraHeaders) > 0 {
		out["extra_headers"] = s.ExtraHeaders
	}
	return out
}

// fragmentInstructions returns the fragment's instructions, falling back to
// any extra-layer instructions value (string or list) already in the doc —
// which the list semantics replace, not duplicate.
func fragmentInstructions(frag *agentv1alpha1.AgentSpecFragment, doc map[string]any) []string {
	if frag != nil && len(frag.Instructions) > 0 {
		return frag.Instructions
	}
	switch v := doc["instructions"].(type) {
	case string:
		delete(doc, "instructions")
		return []string{v}
	case []any:
		delete(doc, "instructions")
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func fragmentSetsToolTimeout(frag *agentv1alpha1.AgentSpecFragment) bool {
	if frag == nil || frag.Extra == nil {
		return false
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(frag.Extra.Raw, &extra); err != nil {
		return false
	}
	_, ok := extra["tool_timeout"]
	return ok
}

// capabilityList returns the doc's capability entries (never nil).
func capabilityList(doc map[string]any) []any {
	if list, ok := doc["capabilities"].([]any); ok {
		return list
	}
	return []any{}
}

// capabilityEntry builds the pinned pydantic-ai capability-spec form:
// a bare name with no config, {"Name": config} otherwise.
func capabilityEntry(name string, config map[string]any) any {
	if len(config) == 0 {
		return name
	}
	return map[string]any{name: config}
}

// hashJSON wraps the domain hash for compiled documents.
func hashJSON(doc map[string]any) (string, error) {
	return hash.JSONStruct(doc)
}

// canonicalize round-trips the document through JSON so every value has its
// generic form (numbers as json.Number, slices as []any): what the schema
// validator checks is byte-for-byte what gets hashed and serialized.
func canonicalize(doc map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
