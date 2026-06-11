package fakes

import (
	"context"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// FakeConfigMapRepo implements repo.ConfigMapRepo for testing.
type FakeConfigMapRepo struct {
	mu         sync.RWMutex
	ConfigMaps map[types.NamespacedName]*corev1.ConfigMap
}

func NewFakeConfigMapRepo() *FakeConfigMapRepo {
	return &FakeConfigMapRepo{ConfigMaps: make(map[types.NamespacedName]*corev1.ConfigMap)}
}

func (f *FakeConfigMapRepo) GetConfigMap(_ context.Context, key types.NamespacedName) (*corev1.ConfigMap, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	cm, ok := f.ConfigMaps[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, key.Name)
	}
	return cm.DeepCopy(), nil
}

func (f *FakeConfigMapRepo) EnsureConfigMap(_ context.Context, desired *corev1.ConfigMap) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	f.ConfigMaps[key] = desired.DeepCopy()
	return nil
}

func (f *FakeConfigMapRepo) DeleteConfigMap(_ context.Context, key types.NamespacedName) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.ConfigMaps, key)
	return nil
}

// FakeDeploymentRepo implements repo.DeploymentRepo for testing.
type FakeDeploymentRepo struct {
	mu          sync.RWMutex
	Deployments map[types.NamespacedName]*appsv1.Deployment
}

func NewFakeDeploymentRepo() *FakeDeploymentRepo {
	return &FakeDeploymentRepo{Deployments: make(map[types.NamespacedName]*appsv1.Deployment)}
}

func (f *FakeDeploymentRepo) EnsureDeployment(_ context.Context, desired *appsv1.Deployment) (*appsv1.Deployment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	f.Deployments[key] = desired.DeepCopy()
	return desired.DeepCopy(), nil
}

// FakeServiceRepo implements repo.ServiceRepo for testing.
type FakeServiceRepo struct {
	mu       sync.RWMutex
	Services map[types.NamespacedName]*corev1.Service
}

func NewFakeServiceRepo() *FakeServiceRepo {
	return &FakeServiceRepo{Services: make(map[types.NamespacedName]*corev1.Service)}
}

func (f *FakeServiceRepo) EnsureService(_ context.Context, desired *corev1.Service) (*corev1.Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	f.Services[key] = desired.DeepCopy()
	return desired.DeepCopy(), nil
}

func (f *FakeServiceRepo) GetService(_ context.Context, key types.NamespacedName) (*corev1.Service, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	svc, ok := f.Services[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, key.Name)
	}
	return svc.DeepCopy(), nil
}

// FakeAgentToolRepo implements repo.AgentToolReader and repo.AgentToolWriter.
type FakeAgentToolRepo struct {
	mu         sync.RWMutex
	AgentTools map[types.NamespacedName]*agentv1alpha1.AgentTool
}

func NewFakeAgentToolRepo() *FakeAgentToolRepo {
	return &FakeAgentToolRepo{AgentTools: make(map[types.NamespacedName]*agentv1alpha1.AgentTool)}
}

func (f *FakeAgentToolRepo) GetAgentTool(_ context.Context, key types.NamespacedName) (*agentv1alpha1.AgentTool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	tool, ok := f.AgentTools[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "agenttools"}, key.Name)
	}
	return tool.DeepCopy(), nil
}

func (f *FakeAgentToolRepo) EnsureAgentTool(_ context.Context, desired *agentv1alpha1.AgentTool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	f.AgentTools[key] = desired.DeepCopy()
	return nil
}

// FakeModelRepo implements repo.ModelReader.
type FakeModelRepo struct {
	mu     sync.RWMutex
	Models map[types.NamespacedName]*agentv1alpha1.Model
}

func NewFakeModelRepo() *FakeModelRepo {
	return &FakeModelRepo{Models: make(map[types.NamespacedName]*agentv1alpha1.Model)}
}

func (f *FakeModelRepo) GetModel(_ context.Context, key types.NamespacedName) (*agentv1alpha1.Model, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	model, ok := f.Models[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "models"}, key.Name)
	}
	return model.DeepCopy(), nil
}

// FakeModelProviderRepo implements repo.ModelProviderReader.
type FakeModelProviderRepo struct {
	mu        sync.RWMutex
	Providers map[types.NamespacedName]*agentv1alpha1.ModelProvider
}

func NewFakeModelProviderRepo() *FakeModelProviderRepo {
	return &FakeModelProviderRepo{Providers: make(map[types.NamespacedName]*agentv1alpha1.ModelProvider)}
}

func (f *FakeModelProviderRepo) GetModelProvider(_ context.Context, key types.NamespacedName) (*agentv1alpha1.ModelProvider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	provider, ok := f.Providers[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "modelproviders"}, key.Name)
	}
	return provider.DeepCopy(), nil
}

// FakeInstructionRepo implements repo.InstructionReader and repo.InstructionWriter.
type FakeInstructionRepo struct {
	mu           sync.RWMutex
	Instructions map[types.NamespacedName]*agentv1alpha1.Instruction
}

func NewFakeInstructionRepo() *FakeInstructionRepo {
	return &FakeInstructionRepo{Instructions: make(map[types.NamespacedName]*agentv1alpha1.Instruction)}
}

func (f *FakeInstructionRepo) GetInstruction(_ context.Context, key types.NamespacedName) (*agentv1alpha1.Instruction, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	instruction, ok := f.Instructions[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "instructions"}, key.Name)
	}
	return instruction.DeepCopy(), nil
}

func (f *FakeInstructionRepo) EnsureInstruction(_ context.Context, desired *agentv1alpha1.Instruction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}
	f.Instructions[key] = desired.DeepCopy()
	return nil
}

// FakeSecretRepo implements repo.SecretReader.
type FakeSecretRepo struct {
	mu      sync.RWMutex
	Secrets map[types.NamespacedName]*corev1.Secret
}

func NewFakeSecretRepo() *FakeSecretRepo {
	return &FakeSecretRepo{Secrets: make(map[types.NamespacedName]*corev1.Secret)}
}

func (f *FakeSecretRepo) GetSecret(_ context.Context, key types.NamespacedName) (*corev1.Secret, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	secret, ok := f.Secrets[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
	}
	return secret.DeepCopy(), nil
}

// FakeAgentRepo implements repo.AgentReader for testing.
type FakeAgentRepo struct {
	mu     sync.RWMutex
	Agents map[types.NamespacedName]*agentv1alpha1.Agent
}

func NewFakeAgentRepo() *FakeAgentRepo {
	return &FakeAgentRepo{Agents: make(map[types.NamespacedName]*agentv1alpha1.Agent)}
}

func (f *FakeAgentRepo) GetAgent(_ context.Context, key types.NamespacedName) (*agentv1alpha1.Agent, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	agent, ok := f.Agents[key]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "agents"}, key.Name)
	}
	return agent.DeepCopy(), nil
}

// FakeOwnerSetter implements repo.OwnerSetter for testing (no-op).
type FakeOwnerSetter struct {
	SetOwnerErr error
}

func (f *FakeOwnerSetter) SetOwner(_, _ metav1.Object) error {
	if f.SetOwnerErr != nil {
		return f.SetOwnerErr
	}
	return nil
}
