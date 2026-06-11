package converter

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

const testFragmentModel = "openai:gpt-5-mini"

// minimalAgentSpec returns the smallest valid AgentSpec: a card plus an
// inline fragment that names a model.
func minimalAgentSpec() agentv1alpha1.AgentSpec {
	return agentv1alpha1.AgentSpec{
		Card: agentv1alpha1.AgentCardOverride{
			Name:        "Test Agent",
			Description: "A test agent",
			Version:     "1.0.0",
		},
		Spec: &agentv1alpha1.AgentSpecFragment{
			Model: testFragmentModel,
		},
	}
}

func TestAgentToProto_Nil(t *testing.T) {
	result := AgentToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToProto_FullAgent(t *testing.T) {
	replicas := int32(3)
	optional := true
	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-agent",
			Namespace: "production",
		},
		Spec: agentv1alpha1.AgentSpec{
			Card: agentv1alpha1.AgentCardOverride{
				Name:        "Test Agent",
				Description: "A test agent",
				Version:     "1.0.0",
			},
			Spec: &agentv1alpha1.AgentSpecFragment{
				Model: testFragmentModel,
			},
			ModelRef: &agentv1alpha1.NamespacedRef{
				Name:      "gpt-5-mini",
				Namespace: "models",
			},
			InstructionRefs: []agentv1alpha1.NamespacedRef{
				{Name: "base-prompt"},
			},
			Tools: []agentv1alpha1.NamespacedRef{
				{Name: "weather-tool", Namespace: "tools"},
			},
			SecretRefs: map[string]corev1.SecretKeySelector{
				"OPENAI_API_KEY": {
					LocalObjectReference: corev1.LocalObjectReference{Name: "openai-secret"},
					Key:                  "api-key",
					Optional:             &optional,
				},
			},
			Runtime: agentv1alpha1.AgentRuntime{
				Isolation: agentv1alpha1.IsolationShared,
				DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
					Replicas: &replicas,
				},
			},
		},
		Status: agentv1alpha1.AgentStatus{
			Phase:             agentv1alpha1.AgentPhaseRunning,
			URL:               "https://my-agent.example.com",
			SpecHash:          "sha256:abc123",
			RunnerVersion:     "0.4.0",
			Replicas:          3,
			AvailableReplicas: 2,
		},
	}

	result := AgentToProto(agent)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Metadata == nil || result.Metadata.Name != "my-agent" {
		t.Fatal("expected metadata with name my-agent")
	}

	if result.Spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if result.Spec.Card == nil || result.Spec.Card.Name != "Test Agent" {
		t.Fatal("expected card with name Test Agent")
	}
	if result.Spec.ModelRef == nil || result.Spec.ModelRef.Name != "gpt-5-mini" {
		t.Fatal("expected model ref gpt-5-mini")
	}

	if result.Status == nil {
		t.Fatal("expected non-nil status")
	}
	if result.Status.Phase != pb.AgentPhase_AGENT_PHASE_RUNNING {
		t.Fatalf("expected running phase, got %v", result.Status.Phase)
	}
}

func TestAgentSpecToProto_Nil(t *testing.T) {
	result := AgentSpecToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentSpecToProto_Minimal(t *testing.T) {
	spec := minimalAgentSpec()

	result := AgentSpecToProto(&spec)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Card == nil {
		t.Fatal("expected card to be set")
	}
	if result.Card.Name != "Test Agent" {
		t.Fatalf("expected card name Test Agent, got %q", result.Card.Name)
	}
	if result.Spec == nil {
		t.Fatal("expected fragment struct to be set")
	}
	if got := result.Spec.Fields["model"].GetStringValue(); got != testFragmentModel {
		t.Fatalf("expected fragment model %q, got %q", testFragmentModel, got)
	}
	if result.ModelRef != nil {
		t.Fatal("expected nil model ref")
	}
	if len(result.InstructionRefs) != 0 {
		t.Fatalf("expected no instruction refs, got %d", len(result.InstructionRefs))
	}
	if len(result.Tools) != 0 {
		t.Fatalf("expected no tools, got %d", len(result.Tools))
	}
	if len(result.Capabilities) != 0 {
		t.Fatalf("expected no capabilities, got %d", len(result.Capabilities))
	}
	if len(result.SecretRefs) != 0 {
		t.Fatalf("expected no secret refs, got %d", len(result.SecretRefs))
	}
}

func TestAgentSpecToProto_NilFragment(t *testing.T) {
	spec := &agentv1alpha1.AgentSpec{
		Card: agentv1alpha1.AgentCardOverride{
			Name:        "No Fragment",
			Description: "Card only",
			Version:     "0.1.0",
		},
	}

	result := AgentSpecToProto(spec)
	if result.Spec != nil {
		t.Fatal("expected nil fragment struct for nil inline spec")
	}
}

func TestAgentSpecToProto_FragmentContent(t *testing.T) {
	spec := &agentv1alpha1.AgentSpec{
		Card: agentv1alpha1.AgentCardOverride{Name: "A", Description: "B", Version: "1"},
		Spec: &agentv1alpha1.AgentSpecFragment{
			Model:        testFragmentModel,
			Name:         "custom-name",
			Instructions: []string{"Be helpful"},
		},
	}

	result := AgentSpecToProto(spec)
	if result.Spec == nil {
		t.Fatal("expected fragment struct to be set")
	}
	if got := result.Spec.Fields["model"].GetStringValue(); got != testFragmentModel {
		t.Fatalf("expected model %q, got %q", testFragmentModel, got)
	}
	if got := result.Spec.Fields["name"].GetStringValue(); got != "custom-name" {
		t.Fatalf("expected name custom-name, got %q", got)
	}
	instructions := result.Spec.Fields["instructions"].GetListValue()
	if instructions == nil || len(instructions.Values) != 1 {
		t.Fatal("expected 1 instruction in fragment struct")
	}
	if got := instructions.Values[0].GetStringValue(); got != "Be helpful" {
		t.Fatalf("expected instruction, got %q", got)
	}
	// Unset optional fields must not leak into the struct.
	if _, ok := result.Spec.Fields["description"]; ok {
		t.Fatal("expected description to be omitted from fragment struct")
	}
}

func TestAgentSpecToProto_Refs(t *testing.T) {
	spec := &agentv1alpha1.AgentSpec{
		Card: agentv1alpha1.AgentCardOverride{Name: "A", Description: "B", Version: "1"},
		ModelRef: &agentv1alpha1.NamespacedRef{
			Name:      "gpt-5-mini",
			Namespace: "models",
		},
		InstructionRefs: []agentv1alpha1.NamespacedRef{
			{Name: "base-prompt", Namespace: "prompts"},
			{Name: "tone"},
		},
		Tools: []agentv1alpha1.NamespacedRef{
			{Name: "weather-tool", Namespace: "tools"},
		},
	}

	result := AgentSpecToProto(spec)
	if result.ModelRef == nil || result.ModelRef.Name != "gpt-5-mini" || result.ModelRef.Namespace != "models" {
		t.Fatalf("expected model ref models/gpt-5-mini, got %v", result.ModelRef)
	}
	if len(result.InstructionRefs) != 2 {
		t.Fatalf("expected 2 instruction refs, got %d", len(result.InstructionRefs))
	}
	if result.InstructionRefs[0].Name != "base-prompt" || result.InstructionRefs[0].Namespace != "prompts" {
		t.Fatalf("expected first instruction ref prompts/base-prompt, got %v", result.InstructionRefs[0])
	}
	if result.InstructionRefs[1].Name != "tone" || result.InstructionRefs[1].Namespace != "" {
		t.Fatalf("expected second instruction ref tone, got %v", result.InstructionRefs[1])
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool ref, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "weather-tool" || result.Tools[0].Namespace != "tools" {
		t.Fatalf("expected tool ref tools/weather-tool, got %v", result.Tools[0])
	}
}

func TestAgentSpecToProto_Capabilities(t *testing.T) {
	spec := &agentv1alpha1.AgentSpec{
		Card: agentv1alpha1.AgentCardOverride{Name: "A", Description: "B", Version: "1"},
		Capabilities: []agentv1alpha1.CapabilityAttachment{
			{
				Ref:    agentv1alpha1.NamespacedRef{Name: "web-search", Namespace: "caps"},
				Config: &apiextensionsv1.JSON{Raw: []byte(`{"maxResults":5}`)},
			},
			{
				Ref: agentv1alpha1.NamespacedRef{Name: "no-config"},
			},
		},
	}

	result := AgentSpecToProto(spec)
	if len(result.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(result.Capabilities))
	}
	first := result.Capabilities[0]
	if first.Ref == nil || first.Ref.Name != "web-search" || first.Ref.Namespace != "caps" {
		t.Fatalf("expected capability ref caps/web-search, got %v", first.Ref)
	}
	if first.Config == nil {
		t.Fatal("expected capability config to be set")
	}
	if got := first.Config.Fields["maxResults"].GetNumberValue(); got != 5 {
		t.Fatalf("expected maxResults 5, got %v", got)
	}
	second := result.Capabilities[1]
	if second.Ref == nil || second.Ref.Name != "no-config" {
		t.Fatalf("expected capability ref no-config, got %v", second.Ref)
	}
	if second.Config != nil {
		t.Fatal("expected nil config for capability without config")
	}
}

func TestAgentSpecToProto_SecretRefs(t *testing.T) {
	optional := true
	spec := &agentv1alpha1.AgentSpec{
		Card: agentv1alpha1.AgentCardOverride{Name: "A", Description: "B", Version: "1"},
		SecretRefs: map[string]corev1.SecretKeySelector{
			"API_KEY": {
				LocalObjectReference: corev1.LocalObjectReference{Name: "api-secret"},
				Key:                  "key",
				Optional:             &optional,
			},
			"OTHER": {
				LocalObjectReference: corev1.LocalObjectReference{Name: "other-secret"},
				Key:                  "token",
			},
		},
	}

	result := AgentSpecToProto(spec)
	if len(result.SecretRefs) != 2 {
		t.Fatalf("expected 2 secret refs, got %d", len(result.SecretRefs))
	}
	apiKey := result.SecretRefs["API_KEY"]
	if apiKey == nil || apiKey.Name != "api-secret" || apiKey.Key != "key" {
		t.Fatalf("expected API_KEY -> api-secret/key, got %v", apiKey)
	}
	if !apiKey.Optional {
		t.Fatal("expected API_KEY optional true")
	}
	other := result.SecretRefs["OTHER"]
	if other == nil || other.Name != "other-secret" || other.Key != "token" {
		t.Fatalf("expected OTHER -> other-secret/token, got %v", other)
	}
	if other.Optional {
		t.Fatal("expected OTHER optional false for nil pointer")
	}
}

func TestNamespacedRefToProto_Nil(t *testing.T) {
	result := NamespacedRefToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestNamespacedRefToProto(t *testing.T) {
	ref := &agentv1alpha1.NamespacedRef{Name: "my-ref", Namespace: "ns"}

	result := NamespacedRefToProto(ref)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "my-ref" {
		t.Fatalf("expected name my-ref, got %q", result.Name)
	}
	if result.Namespace != "ns" {
		t.Fatalf("expected namespace ns, got %q", result.Namespace)
	}
}

func TestAgentCardToProto_Nil(t *testing.T) {
	result := AgentCardToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentCardToProto(t *testing.T) {
	card := &agentv1alpha1.AgentCardOverride{
		Name:               "Test Card",
		Description:        "A test card",
		Version:            "2.0.0",
		DefaultInputModes:  []agentv1alpha1.InputOutputMode{agentv1alpha1.InputOutputModeText, agentv1alpha1.InputOutputModeJSON},
		DefaultOutputModes: []agentv1alpha1.InputOutputMode{agentv1alpha1.InputOutputModeJSON},
		Capabilities: agentv1alpha1.AgentCapabilities{
			PushNotifications:      true,
			StateTransitionHistory: true,
			Streaming:              false,
		},
		Skills: []agentv1alpha1.AgentSkill{
			{
				ID:          "skill-1",
				Name:        "Summarize",
				Description: "Summarizes text",
				Tags:        []string{"nlp", "summary"},
				Examples:    []string{"Summarize this article"},
				InputModes:  []string{"text"},
				OutputModes: []string{"text"},
			},
		},
	}

	result := AgentCardToProto(card)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "Test Card" {
		t.Fatalf("expected name Test Card, got %q", result.Name)
	}
	if result.Description != "A test card" {
		t.Fatalf("expected description, got %q", result.Description)
	}
	if result.Version != "2.0.0" {
		t.Fatalf("expected version 2.0.0, got %q", result.Version)
	}
	if len(result.DefaultInputModes) != 2 {
		t.Fatalf("expected 2 input modes, got %d", len(result.DefaultInputModes))
	}
	if result.DefaultInputModes[0] != pb.InputOutputMode_INPUT_OUTPUT_MODE_TEXT {
		t.Fatalf("expected text input mode, got %v", result.DefaultInputModes[0])
	}
	if result.DefaultInputModes[1] != pb.InputOutputMode_INPUT_OUTPUT_MODE_JSON {
		t.Fatalf("expected json input mode, got %v", result.DefaultInputModes[1])
	}
	if len(result.DefaultOutputModes) != 1 {
		t.Fatalf("expected 1 output mode, got %d", len(result.DefaultOutputModes))
	}
	if result.Capabilities == nil {
		t.Fatal("expected capabilities to be set")
	}
	if !result.Capabilities.PushNotifications {
		t.Fatal("expected push notifications to be true")
	}
	if !result.Capabilities.StateTransitionHistory {
		t.Fatal("expected state transition history to be true")
	}
	if result.Capabilities.Streaming {
		t.Fatal("expected streaming to be false")
	}
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Id != "skill-1" {
		t.Fatalf("expected skill id skill-1, got %q", result.Skills[0].Id)
	}
}

func TestAgentCapabilitiesToProto_Nil(t *testing.T) {
	result := AgentCapabilitiesToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentCapabilitiesToProto(t *testing.T) {
	caps := &agentv1alpha1.AgentCapabilities{
		PushNotifications:      true,
		StateTransitionHistory: false,
		Streaming:              true,
	}

	result := AgentCapabilitiesToProto(caps)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.PushNotifications {
		t.Fatal("expected push notifications true")
	}
	if result.StateTransitionHistory {
		t.Fatal("expected state transition history false")
	}
	if !result.Streaming {
		t.Fatal("expected streaming true")
	}
}

func TestAgentSkillToProto_Nil(t *testing.T) {
	result := AgentSkillToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentSkillToProto(t *testing.T) {
	skill := &agentv1alpha1.AgentSkill{
		ID:          "s1",
		Name:        "Search",
		Description: "Search the web",
		Tags:        []string{"search", "web"},
		Examples:    []string{"Search for Go tutorials"},
		InputModes:  []string{"text"},
		OutputModes: []string{"text", "json"},
	}

	result := AgentSkillToProto(skill)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Id != "s1" {
		t.Fatalf("expected id s1, got %q", result.Id)
	}
	if result.Name != "Search" {
		t.Fatalf("expected name Search, got %q", result.Name)
	}
	if result.Description != "Search the web" {
		t.Fatalf("expected description, got %q", result.Description)
	}
	if len(result.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(result.Tags))
	}
	if len(result.Examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(result.Examples))
	}
	if len(result.InputModes) != 1 {
		t.Fatalf("expected 1 input mode, got %d", len(result.InputModes))
	}
	if len(result.OutputModes) != 2 {
		t.Fatalf("expected 2 output modes, got %d", len(result.OutputModes))
	}
}

func TestAgentRuntimeToProto_Nil(t *testing.T) {
	result := AgentRuntimeToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentRuntimeToProto(t *testing.T) {
	replicas := int32(2)
	runtime := &agentv1alpha1.AgentRuntime{
		Image:         "custom-runner:v1",
		RunnerVersion: "0.4.0",
		Isolation:     agentv1alpha1.IsolationSession,
		Env: []corev1.EnvVar{
			{Name: "LOG_LEVEL", Value: "debug"},
			{Name: "EMPTY"},
		},
		Resources: &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("500m"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
			Replicas:           &replicas,
			ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "pull-secret"}},
			ServiceAccountName: "runner-sa",
			NodeSelector:       map[string]string{"gpu": "true"},
		},
	}

	result := AgentRuntimeToProto(runtime)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Image != "custom-runner:v1" {
		t.Fatalf("expected image custom-runner:v1, got %q", result.Image)
	}
	if result.RunnerVersion != "0.4.0" {
		t.Fatalf("expected runner version 0.4.0, got %q", result.RunnerVersion)
	}
	if result.Isolation != pb.IsolationTier_ISOLATION_TIER_SESSION {
		t.Fatalf("expected session isolation, got %v", result.Isolation)
	}
	if len(result.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(result.Env))
	}
	if result.Env[0].Name != "LOG_LEVEL" || result.Env[0].Value != "debug" {
		t.Fatalf("expected LOG_LEVEL=debug, got %v", result.Env[0])
	}
	if result.Resources == nil {
		t.Fatal("expected resources to be set")
	}
	if result.Resources.Limits["cpu"] != "500m" {
		t.Fatalf("expected cpu limit 500m, got %q", result.Resources.Limits["cpu"])
	}
	if result.Resources.Requests["memory"] != "128Mi" {
		t.Fatalf("expected memory request 128Mi, got %q", result.Resources.Requests["memory"])
	}
	if result.Replicas != 2 {
		t.Fatalf("expected 2 replicas, got %d", result.Replicas)
	}
	if len(result.ImagePullSecrets) != 1 || result.ImagePullSecrets[0].Name != "pull-secret" {
		t.Fatal("expected image pull secret pull-secret")
	}
	if result.ServiceAccountName != "runner-sa" {
		t.Fatalf("expected service account runner-sa, got %q", result.ServiceAccountName)
	}
	if result.NodeSelector["gpu"] != "true" {
		t.Fatal("expected node selector gpu=true")
	}
}

func TestAgentRuntimeToProto_Empty(t *testing.T) {
	result := AgentRuntimeToProto(&agentv1alpha1.AgentRuntime{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Replicas != 0 {
		t.Fatalf("expected 0 replicas for nil pointer, got %d", result.Replicas)
	}
	if result.Isolation != pb.IsolationTier_ISOLATION_TIER_UNSPECIFIED {
		t.Fatalf("expected unspecified isolation, got %v", result.Isolation)
	}
	if result.Resources != nil {
		t.Fatal("expected nil resources")
	}
	if len(result.Env) != 0 {
		t.Fatalf("expected no env vars, got %d", len(result.Env))
	}
	if len(result.ImagePullSecrets) != 0 {
		t.Fatalf("expected no image pull secrets, got %d", len(result.ImagePullSecrets))
	}
}

func TestAgentStatusToProto_Nil(t *testing.T) {
	result := AgentStatusToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentStatusToProto(t *testing.T) {
	status := &agentv1alpha1.AgentStatus{
		Phase:                agentv1alpha1.AgentPhaseRunning,
		URL:                  "https://agent.example.com",
		SpecHash:             "sha256:abc",
		RunnerVersion:        "0.4.0",
		InjectedCapabilities: []string{"MCP", "Thinking"},
		Replicas:             3,
		AvailableReplicas:    2,
		ObservedGeneration:   7,
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
		},
	}

	result := AgentStatusToProto(status)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Phase != pb.AgentPhase_AGENT_PHASE_RUNNING {
		t.Fatalf("expected running phase, got %v", result.Phase)
	}
	if result.Url != "https://agent.example.com" {
		t.Fatalf("expected URL, got %q", result.Url)
	}
	if result.SpecHash != "sha256:abc" {
		t.Fatalf("expected spec hash, got %q", result.SpecHash)
	}
	if result.RunnerVersion != "0.4.0" {
		t.Fatalf("expected runner version 0.4.0, got %q", result.RunnerVersion)
	}
	if len(result.InjectedCapabilities) != 2 || result.InjectedCapabilities[0] != "MCP" {
		t.Fatalf("expected injected capabilities, got %v", result.InjectedCapabilities)
	}
	if result.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", result.Replicas)
	}
	if result.AvailableReplicas != 2 {
		t.Fatalf("expected 2 available replicas, got %d", result.AvailableReplicas)
	}
	if result.ObservedGeneration != 7 {
		t.Fatalf("expected observed gen 7, got %d", result.ObservedGeneration)
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(result.Conditions))
	}
}

func TestAgentListToProto_Nil(t *testing.T) {
	result := AgentListToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentListToProto(t *testing.T) {
	list := &agentv1alpha1.AgentList{
		ListMeta: metav1.ListMeta{ResourceVersion: "100"},
		Items: []agentv1alpha1.Agent{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "agent-1", Namespace: "ns"},
				Spec:       minimalAgentSpec(),
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "agent-2", Namespace: "ns"},
				Spec:       minimalAgentSpec(),
			},
		},
	}

	result := AgentListToProto(list)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil || result.Metadata.ResourceVersion != "100" {
		t.Fatal("expected metadata")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Metadata.Name != "agent-1" {
		t.Fatalf("expected agent-1, got %q", result.Items[0].Metadata.Name)
	}
	if result.Items[1].Metadata.Name != "agent-2" {
		t.Fatalf("expected agent-2, got %q", result.Items[1].Metadata.Name)
	}
}

func TestAgentListToProto_Empty(t *testing.T) {
	list := &agentv1alpha1.AgentList{
		Items: []agentv1alpha1.Agent{},
	}

	result := AgentListToProto(list)
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
}

func TestAgentPhaseToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.AgentPhase
		expected pb.AgentPhase
	}{
		{agentv1alpha1.AgentPhasePending, pb.AgentPhase_AGENT_PHASE_PENDING},
		{agentv1alpha1.AgentPhaseRunning, pb.AgentPhase_AGENT_PHASE_RUNNING},
		{agentv1alpha1.AgentPhaseFailed, pb.AgentPhase_AGENT_PHASE_FAILED},
		{agentv1alpha1.AgentPhase("unknown"), pb.AgentPhase_AGENT_PHASE_UNSPECIFIED},
		{agentv1alpha1.AgentPhase(""), pb.AgentPhase_AGENT_PHASE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := AgentPhaseToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("AgentPhaseToProto(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsolationTierToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.IsolationTier
		expected pb.IsolationTier
	}{
		{agentv1alpha1.IsolationShared, pb.IsolationTier_ISOLATION_TIER_SHARED},
		{agentv1alpha1.IsolationSession, pb.IsolationTier_ISOLATION_TIER_SESSION},
		{agentv1alpha1.IsolationTier("unknown"), pb.IsolationTier_ISOLATION_TIER_UNSPECIFIED},
		{agentv1alpha1.IsolationTier(""), pb.IsolationTier_ISOLATION_TIER_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := IsolationTierToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("IsolationTierToProto(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestInputOutputModeToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.InputOutputMode
		expected pb.InputOutputMode
	}{
		{agentv1alpha1.InputOutputModeText, pb.InputOutputMode_INPUT_OUTPUT_MODE_TEXT},
		{agentv1alpha1.InputOutputModeJSON, pb.InputOutputMode_INPUT_OUTPUT_MODE_JSON},
		{agentv1alpha1.InputOutputMode("unknown"), pb.InputOutputMode_INPUT_OUTPUT_MODE_UNSPECIFIED},
		{agentv1alpha1.InputOutputMode(""), pb.InputOutputMode_INPUT_OUTPUT_MODE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := InputOutputModeToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("InputOutputModeToProto(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
