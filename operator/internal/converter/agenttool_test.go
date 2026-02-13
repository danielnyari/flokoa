package converter

import (
	"encoding/json"
	"reflect"
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestOpenApiSchemaToProtoAndFromProto_WithInlineValue(t *testing.T) {
	rawJSON := []byte(`{"openapi":"3.0.0","info":{"title":"Weather API","version":"1.0.0"},"paths":{"/weather":{"get":{"operationId":"getWeather"}}}}`)

	schema := &agentv1alpha1.OpenApiSchema{
		Value:        &runtime.RawExtension{Raw: rawJSON},
		EndpointPath: "/openapi.json",
	}

	protoSchema := OpenApiSchemaToProto(schema)
	if protoSchema == nil {
		t.Fatal("expected proto schema, got nil")
	}
	if protoSchema.Value == nil {
		t.Fatal("expected proto schema value to be set")
	}
	if protoSchema.EndpointPath != "/openapi.json" {
		t.Fatalf("expected endpoint path /openapi.json, got %q", protoSchema.EndpointPath)
	}

	roundTrip := OpenApiSchemaFromProto(protoSchema)
	if roundTrip == nil {
		t.Fatal("expected round-trip schema, got nil")
	}
	if roundTrip.Value == nil {
		t.Fatal("expected round-trip schema value to be set")
	}

	if !jsonSemanticallyEqual(t, rawJSON, roundTrip.Value.Raw) {
		t.Fatalf("expected round-trip raw JSON to be semantically equal")
	}
}

func TestAgentToolSpecToProtoAndFromProto_OpenApiFields(t *testing.T) {
	timeout := int32(45)
	port := int32(8080)

	spec := &agentv1alpha1.AgentToolSpec{
		Type:        agentv1alpha1.AgentToolTypeOpenAPI,
		Description: "Weather service",
		OpenApi: &agentv1alpha1.OpenApiToolSpec{
			URL: "https://weather.example.com",
			ServiceRef: &agentv1alpha1.ServiceRef{
				Name:      "weather-svc",
				Namespace: "default",
				Port:      &port,
				PortName:  "http",
			},
			OpenApiSchema: agentv1alpha1.OpenApiSchema{
				ValueFrom: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "weather-openapi"},
					Key:                  "spec.json",
				},
				EndpointPath: "/docs/openapi.json",
			},
			TimeoutSeconds: &timeout,
			Headers: map[string]string{
				"Authorization": "Bearer ${API_KEY}",
				"X-Client":      "flokoa",
			},
		},
	}

	protoSpec := AgentToolSpecToProto(spec)
	if protoSpec == nil {
		t.Fatal("expected proto spec, got nil")
	}
	if protoSpec.Type != pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI {
		t.Fatalf("expected openapi type, got %v", protoSpec.Type)
	}
	if protoSpec.OpenApi == nil {
		t.Fatal("expected open_api spec to be set")
	}
	if protoSpec.OpenApi.TimeoutSeconds != 45 {
		t.Fatalf("expected timeout 45, got %d", protoSpec.OpenApi.TimeoutSeconds)
	}
	if protoSpec.OpenApi.OpenApiSchema == nil || protoSpec.OpenApi.OpenApiSchema.ValueFrom == nil {
		t.Fatal("expected open_api_schema.value_from to be set")
	}

	roundTrip := AgentToolSpecFromProto(protoSpec)
	if roundTrip == nil {
		t.Fatal("expected round-trip spec, got nil")
	}
	if roundTrip.Type != agentv1alpha1.AgentToolTypeOpenAPI {
		t.Fatalf("expected openapi type, got %q", roundTrip.Type)
	}
	if roundTrip.OpenApi == nil || roundTrip.OpenApi.ServiceRef == nil {
		t.Fatal("expected openapi and serviceRef to be set")
	}
	if roundTrip.OpenApi.TimeoutSeconds == nil || *roundTrip.OpenApi.TimeoutSeconds != 45 {
		t.Fatal("expected timeoutSeconds to round-trip")
	}
	if roundTrip.OpenApi.OpenApiSchema.ValueFrom == nil {
		t.Fatal("expected valueFrom to round-trip")
	}
	if roundTrip.OpenApi.OpenApiSchema.ValueFrom.Name != "weather-openapi" || roundTrip.OpenApi.OpenApiSchema.ValueFrom.Key != "spec.json" {
		t.Fatal("expected valueFrom name/key to round-trip")
	}
	if !reflect.DeepEqual(roundTrip.OpenApi.Headers, spec.OpenApi.Headers) {
		t.Fatalf("expected headers to round-trip, got %#v", roundTrip.OpenApi.Headers)
	}
}

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
