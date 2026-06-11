package plugin

import (
	"context"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// Resolver resolves agent names to their published A2A endpoint URLs.
type Resolver struct {
	k8sClient client.Client
}

// NewResolver creates a new Resolver with the given Kubernetes client.
func NewResolver(k8sClient client.Client) *Resolver {
	return &Resolver{
		k8sClient: k8sClient,
	}
}

// Resolve resolves an agent to its published endpoint via the Agent CR's
// status.url — the flokoa-owned virtual endpoint. There is deliberately no
// DNS-convention fallback: the URL is an opaque contract whose backing
// topology may change (e.g. the session router), so guessing Service names
// would silently bypass it.
func (r *Resolver) Resolve(ctx context.Context, agent, namespace string) (string, error) {
	if agent == "" {
		return "", fmt.Errorf("agent name is required")
	}
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if r.k8sClient == nil {
		return "", fmt.Errorf("no Kubernetes client configured to resolve agent %s/%s", namespace, agent)
	}

	var agentCR agentv1alpha1.Agent
	key := types.NamespacedName{Name: agent, Namespace: namespace}
	if err := r.k8sClient.Get(ctx, key, &agentCR); err != nil {
		return "", fmt.Errorf("failed to get Agent CR %s/%s: %w", namespace, agent, err)
	}

	if agentCR.Status.URL == "" {
		return "", fmt.Errorf("agent %s/%s has no published endpoint yet (status.url is empty)", namespace, agent)
	}

	log.Printf("Resolved agent endpoint: agent=%s namespace=%s endpoint=%s", agent, namespace, agentCR.Status.URL)
	return agentCR.Status.URL, nil
}
