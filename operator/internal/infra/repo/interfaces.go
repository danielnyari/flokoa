package repo

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ConfigMapRepo handles ConfigMap CRUD operations.
type ConfigMapRepo interface {
	GetConfigMap(ctx context.Context, key types.NamespacedName) (*corev1.ConfigMap, error)
	EnsureConfigMap(ctx context.Context, desired *corev1.ConfigMap) error
	DeleteConfigMap(ctx context.Context, key types.NamespacedName) error
}

// DeploymentRepo handles Deployment get-or-create-or-update operations.
type DeploymentRepo interface {
	EnsureDeployment(ctx context.Context, desired *appsv1.Deployment) (*appsv1.Deployment, error)
}

// ServiceRepo handles Service get-or-create-or-update operations.
type ServiceRepo interface {
	EnsureService(ctx context.Context, desired *corev1.Service) (*corev1.Service, error)
}

// ServiceReader reads Service resources (e.g. to resolve named ports of MCP
// endpoints).
type ServiceReader interface {
	GetService(ctx context.Context, key types.NamespacedName) (*corev1.Service, error)
}

// AgentToolReader reads AgentTool resources.
type AgentToolReader interface {
	GetAgentTool(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.AgentTool, error)
}

// AgentToolWriter creates or updates AgentTool resources.
type AgentToolWriter interface {
	EnsureAgentTool(ctx context.Context, desired *agentv1alpha1.AgentTool) error
}

// ModelReader reads Model resources.
type ModelReader interface {
	GetModel(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Model, error)
}

// ModelProviderReader reads ModelProvider resources.
type ModelProviderReader interface {
	GetModelProvider(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.ModelProvider, error)
}

// InstructionReader reads Instruction resources.
type InstructionReader interface {
	GetInstruction(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Instruction, error)
}

// InstructionWriter creates or updates Instruction resources.
type InstructionWriter interface {
	EnsureInstruction(ctx context.Context, desired *agentv1alpha1.Instruction) error
}

// CapabilityReader reads Capability resources.
type CapabilityReader interface {
	GetCapability(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Capability, error)
}

// SecretReader reads Secret resources.
type SecretReader interface {
	GetSecret(ctx context.Context, key types.NamespacedName) (*corev1.Secret, error)
}

// AgentReader reads Agent resources.
type AgentReader interface {
	GetAgent(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Agent, error)
}

// OwnerSetter sets controller owner references.
type OwnerSetter interface {
	SetOwner(owner, controlled metav1.Object) error
}
