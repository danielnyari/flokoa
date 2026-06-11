package plugin

import (
	"context"
	"strings"
	"testing"
)

func TestResolveRequiresClient(t *testing.T) {
	r := NewResolver(nil)
	_, err := r.Resolve(context.Background(), "petstore-agent", "flokoa-system")
	if err == nil || !strings.Contains(err.Error(), "no Kubernetes client") {
		t.Fatalf("expected missing-client error (no DNS-convention fallback), got %v", err)
	}
}

func TestResolveValidatesInputs(t *testing.T) {
	r := NewResolver(nil)
	if _, err := r.Resolve(context.Background(), "", "ns"); err == nil {
		t.Fatal("expected error for empty agent name")
	}
	if _, err := r.Resolve(context.Background(), "agent", ""); err == nil {
		t.Fatal("expected error for empty namespace")
	}
}
