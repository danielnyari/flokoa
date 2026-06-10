package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/PaesslerAG/jsonpath"
)

// ExtractSessionKey evaluates a JSONPath expression against the event data
// and returns a deterministic session ID (SHA256 hash of the extracted value).
// Returns empty string if the expression is empty or yields no value.
func ExtractSessionKey(sessionKeyFrom string, eventData json.RawMessage) string {
	if sessionKeyFrom == "" {
		return ""
	}

	// Parse event data into interface{} for JSONPath evaluation
	var data interface{}
	if err := json.Unmarshal(eventData, &data); err != nil {
		return ""
	}

	result, err := jsonpath.Get(sessionKeyFrom, data)
	if err != nil {
		return ""
	}

	// Convert result to string for hashing
	var valueStr string
	switch v := result.(type) {
	case string:
		valueStr = v
	case float64:
		valueStr = fmt.Sprintf("%v", v)
	case bool:
		valueStr = fmt.Sprintf("%v", v)
	default:
		// For complex types, JSON-marshal
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		valueStr = string(b)
	}

	if valueStr == "" {
		return ""
	}

	// Hash to deterministic contextId
	h := sha256.Sum256([]byte(valueStr))
	return hex.EncodeToString(h[:16]) // 32-char hex
}
