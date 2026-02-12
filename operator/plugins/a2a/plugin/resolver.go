package plugin

import (
	"context"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// Resolver resolves agent names to their A2A endpoint URLs
type Resolver struct {
	k8sClient client.Client
}

// NewResolver creates a new Resolver with the given Kubernetes client
// If k8sClient is nil, the resolver will only use convention-based resolution
func NewResolver(k8sClient client.Client) *Resolver {
	return &Resolver{
		k8sClient: k8sClient,
	}
}

// Resolve resolves an agent name and namespace to its A2A endpoint URL.
// It first attempts to read the endpoint from the Agent CR's status.url field.
// If that fails or is empty, it falls back to a convention-based URL.
func (r *Resolver) Resolve(ctx context.Context, agent, namespace string) (string, error) {
	if agent == "" {
		return "", fmt.Errorf("agent name is required")
	}
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}

	// Try to resolve from Agent CR status
	if r.k8sClient != nil {
		endpoint, err := r.resolveFromCR(ctx, agent, namespace)
		if err == nil && endpoint != "" {
			log.Printf("Resolved agent endpoint from CR: agent=%s namespace=%s endpoint=%s", agent, namespace, endpoint)
			return endpoint, nil
		}
		log.Printf("Warning: failed to resolve endpoint from CR for agent=%s namespace=%s: %v", agent, namespace, err)
	}

	// Fallback to convention-based URL
	log.Printf("Using convention-based endpoint for agent=%s namespace=%s", agent, namespace)
	return r.conventionBasedURL(agent, namespace), nil
}

// resolveFromCR attempts to get the endpoint from the Agent CR's status
func (r *Resolver) resolveFromCR(ctx context.Context, agent, namespace string) (string, error) {
	var agentCR agentv1alpha1.Agent
	key := types.NamespacedName{
		Name:      agent,
		Namespace: namespace,
	}

	if err := r.k8sClient.Get(ctx, key, &agentCR); err != nil {
		return "", fmt.Errorf("failed to get Agent CR: %w", err)
	}

	if agentCR.Status.URL == "" {
		return "", fmt.Errorf("Agent CR has no URL in status")
	}

	return agentCR.Status.URL, nil
}

// conventionBasedURL returns the convention-based A2A endpoint URL
func (r *Resolver) conventionBasedURL(agent, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local/", agent, namespace)
}
