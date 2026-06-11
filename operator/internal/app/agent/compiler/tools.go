package compiler

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/spec"
)

// compileTool turns an AgentTool (declarative MCP endpoint) into a capability
// entry of the compiled spec, plus the env projections for its header
// secrets. Header secret values never enter the spec — only ${secret:…}
// placeholders the runner resolves from FLOKOA_SECRET_* env.
func (c *Compiler) compileTool(ctx context.Context, tool *agentv1alpha1.AgentTool) (any, []corev1.EnvVar, error) {
	url, err := c.resolveToolURL(ctx, tool)
	if err != nil {
		return nil, nil, err
	}

	mcp := map[string]any{
		"url": url,
		"id":  tool.Name,
		// The agent pod connects to the MCP server itself: in-cluster
		// endpoints are not reachable from model providers' native MCP
		// support.
		"native": false,
		"local":  true,
	}
	if tool.Spec.Description != "" {
		mcp["description"] = tool.Spec.Description
	}

	headers := map[string]any{}
	for name, value := range tool.Spec.Headers {
		headers[name] = value
	}

	secretEnv := make([]corev1.EnvVar, 0, len(tool.Spec.HeaderSecrets))
	for _, hs := range tool.Spec.HeaderSecrets {
		refName := toolSecretRefName(tool.Name, hs.Name)
		headers[hs.Name] = spec.SecretPlaceholder(refName)
		selector := hs.SecretRef
		secretEnv = append(secretEnv, corev1.EnvVar{
			Name:      spec.SecretEnvName(refName),
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &selector},
		})
	}
	if len(headers) > 0 {
		mcp["headers"] = headers
	}

	if len(tool.Spec.AllowedTools) > 0 {
		mcp["allowed_tools"] = tool.Spec.AllowedTools
	}

	var entry any = map[string]any{"MCP": mcp}
	if tool.Spec.ToolPrefix != "" {
		entry = map[string]any{"PrefixTools": map[string]any{
			"prefix":     tool.Spec.ToolPrefix,
			"capability": entry,
		}}
	}
	return entry, secretEnv, nil
}

// toolSecretRefName mints the compiler-derived secret reference name for an
// AgentTool header secret (contract §3 grammar).
func toolSecretRefName(toolName, headerName string) string {
	return fmt.Sprintf("tool-%s-%s", toolName, strings.ToLower(headerName))
}

func (c *Compiler) resolveToolURL(ctx context.Context, tool *agentv1alpha1.AgentTool) (string, error) {
	if tool.Spec.URL != "" {
		return tool.Spec.URL, nil
	}

	ref := tool.Spec.ServiceRef
	if ref == nil {
		return "", flokoaerrors.NewPermanentf("AgentTool %s/%s has neither url nor serviceRef", tool.Namespace, tool.Name)
	}
	ns := defaultNS(ref.Namespace, tool.Namespace)

	port := int32(80)
	switch {
	case ref.Port != nil:
		port = *ref.Port
	case ref.PortName != "":
		svc, err := c.deps.Services.GetService(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns})
		if err != nil {
			return "", flokoaerrors.NewDependencyf(
				"Service %s/%s (for AgentTool %s/%s port name %q) not found: %v",
				ns, ref.Name, tool.Namespace, tool.Name, ref.PortName, err)
		}
		found := false
		for _, p := range svc.Spec.Ports {
			if p.Name == ref.PortName {
				port = p.Port
				found = true
				break
			}
		}
		if !found {
			return "", flokoaerrors.NewDependencyf(
				"Service %s/%s has no port named %q (AgentTool %s/%s)",
				ns, ref.Name, ref.PortName, tool.Namespace, tool.Name)
		}
	}

	path := tool.Spec.Path
	if path == "" {
		if tool.Spec.Transport == agentv1alpha1.MCPTransportSSE {
			path = "/sse"
		} else {
			path = "/mcp"
		}
	}

	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s", ref.Name, ns, port, path), nil
}
