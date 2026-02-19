package converter

import (
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testModelName = "gpt-4o"

func TestAgentToProto_Nil(t *testing.T) {
	result := AgentToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToProto_FullAgent(t *testing.T) {
	replicas := int32(3)
	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-agent",
			Namespace: "production",
		},
		Spec: agentv1alpha1.AgentSpec{
			Framework: agentv1alpha1.FrameworkPydanticAI,
			CardOverride: agentv1alpha1.AgentCardOverride{
				Name:        "Test Agent",
				Description: "A test agent",
				Version:     "1.0.0",
			},
			Runtime: agentv1alpha1.RuntimeSpec{
				Type: agentv1alpha1.RuntimeTypeStandard,
				Standard: &agentv1alpha1.StandardRuntimeSpec{
					DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
						Replicas: &replicas,
					},
					Container: corev1.Container{
						Name:  "agent",
						Image: "my-agent:latest",
					},
				},
			},
			Model: &agentv1alpha1.AgentModelRef{
				Name:      testModelName,
				Namespace: "models",
			},
			Tools: []agentv1alpha1.ToolEntry{
				{
					Name: "weather",
					ToolRef: &agentv1alpha1.ToolRef{
						Name:      "weather-tool",
						Namespace: "tools",
					},
				},
			},
		},
		Status: agentv1alpha1.AgentStatus{
			Phase:             agentv1alpha1.AgentPhaseRunning,
			Backend:           "standard",
			URL:               "https://my-agent.example.com",
			Replicas:          3,
			AvailableReplicas: 2,
		},
	}

	result := AgentToProto(agent)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Check metadata
	if result.Metadata == nil || result.Metadata.Name != "my-agent" {
		t.Fatal("expected metadata with name my-agent")
	}

	// Check spec
	if result.Spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if result.Spec.Framework != pb.Framework_FRAMEWORK_PYDANTIC_AI {
		t.Fatalf("expected pydantic-ai framework, got %v", result.Spec.Framework)
	}

	// Check status
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

func TestAgentSpecToProto_WithModel(t *testing.T) {
	spec := &agentv1alpha1.AgentSpec{
		Framework: agentv1alpha1.FrameworkLangChain,
		Model: &agentv1alpha1.AgentModelRef{
			Name:      testModelName,
			Namespace: "models",
		},
	}

	result := AgentSpecToProto(spec)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Model == nil {
		t.Fatal("expected model ref to be set")
	}
	if result.Model.Name != testModelName {
		t.Fatalf("expected model name gpt-4o, got %q", result.Model.Name)
	}
	if result.Model.Namespace != "models" {
		t.Fatalf("expected model namespace models, got %q", result.Model.Namespace)
	}
}

func TestAgentSpecToProto_WithoutModel(t *testing.T) {
	spec := &agentv1alpha1.AgentSpec{
		Framework: agentv1alpha1.FrameworkPydanticAI,
	}

	result := AgentSpecToProto(spec)
	if result.Model != nil {
		t.Fatal("expected nil model ref")
	}
}

func TestAgentSpecToProto_WithTools(t *testing.T) {
	spec := &agentv1alpha1.AgentSpec{
		Tools: []agentv1alpha1.ToolEntry{
			{
				Name: "tool1",
				ToolRef: &agentv1alpha1.ToolRef{
					Name: "ref-tool",
				},
			},
			{
				Name: "tool2",
				Template: &agentv1alpha1.AgentToolSpec{
					Type:        agentv1alpha1.AgentToolTypeOpenAPI,
					Description: "inline tool",
				},
			},
		},
	}

	result := AgentSpecToProto(spec)
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
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

func TestRuntimeSpecToProto_Nil(t *testing.T) {
	result := RuntimeSpecToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestRuntimeSpecToProto_Standard(t *testing.T) {
	replicas := int32(2)
	runtime := &agentv1alpha1.RuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeStandard,
		Standard: &agentv1alpha1.StandardRuntimeSpec{
			DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
				Replicas:           &replicas,
				ServiceAccountName: "my-sa",
				NodeSelector:       map[string]string{"gpu": "true"},
			},
			Container: corev1.Container{
				Name:            "agent",
				Image:           "agent:v1",
				Command:         []string{"python", "-m", "agent"},
				Args:            []string{"--port", "8080"},
				WorkingDir:      "/app",
				ImagePullPolicy: corev1.PullAlways,
			},
		},
	}

	result := RuntimeSpecToProto(runtime)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Type != pb.RuntimeType_RUNTIME_TYPE_STANDARD {
		t.Fatalf("expected standard runtime type, got %v", result.Type)
	}
	if result.Spec == nil {
		t.Fatal("expected spec to be set")
	}
}

func TestRuntimeSpecToProto_NoStandard(t *testing.T) {
	runtime := &agentv1alpha1.RuntimeSpec{
		Type: agentv1alpha1.RuntimeTypeStandard,
	}

	result := RuntimeSpecToProto(runtime)
	if result.Spec != nil {
		t.Fatal("expected nil spec for nil standard")
	}
}

func TestStandardRuntimeSpecToProto_Nil(t *testing.T) {
	result := StandardRuntimeSpecToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestStandardRuntimeSpecToProto(t *testing.T) {
	replicas := int32(5)
	spec := &agentv1alpha1.StandardRuntimeSpec{
		DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
			Replicas:           &replicas,
			ServiceAccountName: "sa-name",
			NodeSelector:       map[string]string{"zone": "us-east-1a"},
		},
		Container: corev1.Container{
			Name:            "container",
			Image:           "image:tag",
			Command:         []string{"/bin/sh"},
			Args:            []string{"-c", "echo hello"},
			WorkingDir:      "/workspace",
			ImagePullPolicy: corev1.PullIfNotPresent,
		},
	}

	result := StandardRuntimeSpecToProto(spec)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %d", result.Replicas)
	}
	if result.ServiceAccountName != "sa-name" {
		t.Fatalf("expected sa-name, got %q", result.ServiceAccountName)
	}
	if result.NodeSelector["zone"] != "us-east-1a" {
		t.Fatal("expected node selector zone=us-east-1a")
	}
	if result.Container == nil {
		t.Fatal("expected container to be set")
	}
	if result.Container.Name != "container" {
		t.Fatalf("expected container name, got %q", result.Container.Name)
	}
	if result.Container.Image != "image:tag" {
		t.Fatalf("expected image, got %q", result.Container.Image)
	}
	if len(result.Container.Command) != 1 || result.Container.Command[0] != "/bin/sh" {
		t.Fatal("expected command")
	}
	if len(result.Container.Args) != 2 {
		t.Fatal("expected args")
	}
	if result.Container.WorkingDir != "/workspace" {
		t.Fatalf("expected working dir, got %q", result.Container.WorkingDir)
	}
	if result.Container.ImagePullPolicy != "IfNotPresent" {
		t.Fatalf("expected IfNotPresent, got %q", result.Container.ImagePullPolicy)
	}
}

func TestStandardRuntimeSpecToProto_NilReplicas(t *testing.T) {
	spec := &agentv1alpha1.StandardRuntimeSpec{
		Container: corev1.Container{
			Name:  "c",
			Image: "img:v1",
		},
	}

	result := StandardRuntimeSpecToProto(spec)
	if result.Replicas != 0 {
		t.Fatalf("expected 0 replicas for nil, got %d", result.Replicas)
	}
}

func TestToolEntryToProto_Nil(t *testing.T) {
	result := ToolEntryToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestToolEntryToProto_WithToolRef(t *testing.T) {
	entry := &agentv1alpha1.ToolEntry{
		Name: "my-tool",
		ToolRef: &agentv1alpha1.ToolRef{
			Name:      "weather-tool",
			Namespace: "tools",
		},
	}

	result := ToolEntryToProto(entry)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "my-tool" {
		t.Fatalf("expected name my-tool, got %q", result.Name)
	}
	toolRef, ok := result.Tool.(*pb.ToolEntry_ToolRef)
	if !ok {
		t.Fatal("expected ToolRef variant")
	}
	if toolRef.ToolRef.Name != "weather-tool" {
		t.Fatalf("expected weather-tool, got %q", toolRef.ToolRef.Name)
	}
	if toolRef.ToolRef.Namespace != "tools" {
		t.Fatalf("expected tools namespace, got %q", toolRef.ToolRef.Namespace)
	}
}

func TestToolEntryToProto_WithInlineTemplate(t *testing.T) {
	entry := &agentv1alpha1.ToolEntry{
		Name: "inline-tool",
		Template: &agentv1alpha1.AgentToolSpec{
			Type:        agentv1alpha1.AgentToolTypeOpenAPI,
			Description: "An inline tool",
		},
	}

	result := ToolEntryToProto(entry)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	inline, ok := result.Tool.(*pb.ToolEntry_Inline)
	if !ok {
		t.Fatal("expected Inline variant")
	}
	if inline.Inline.Type != pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI {
		t.Fatalf("expected openapi type, got %v", inline.Inline.Type)
	}
	if inline.Inline.Description != "An inline tool" {
		t.Fatalf("expected description, got %q", inline.Inline.Description)
	}
}

func TestToolEntryToProto_NoRefNoTemplate(t *testing.T) {
	entry := &agentv1alpha1.ToolEntry{
		Name: "empty-tool",
	}

	result := ToolEntryToProto(entry)
	if result.Tool != nil {
		t.Fatal("expected nil tool variant when neither ref nor template set")
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
		Phase:              agentv1alpha1.AgentPhaseRunning,
		Backend:            "standard",
		URL:                "https://agent.example.com",
		Replicas:           3,
		AvailableReplicas:  2,
		DetectedFramework:  agentv1alpha1.FrameworkLangChain,
		ObservedGeneration: 7,
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
	if result.Backend != "standard" {
		t.Fatalf("expected standard backend, got %q", result.Backend)
	}
	if result.Url != "https://agent.example.com" {
		t.Fatalf("expected URL, got %q", result.Url)
	}
	if result.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", result.Replicas)
	}
	if result.AvailableReplicas != 2 {
		t.Fatalf("expected 2 available replicas, got %d", result.AvailableReplicas)
	}
	if result.DetectedFramework != pb.Framework_FRAMEWORK_LANGCHAIN {
		t.Fatalf("expected langchain framework, got %v", result.DetectedFramework)
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
				Spec:       agentv1alpha1.AgentSpec{Framework: agentv1alpha1.FrameworkPydanticAI},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "agent-2", Namespace: "ns"},
				Spec:       agentv1alpha1.AgentSpec{Framework: agentv1alpha1.FrameworkA2A},
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

func TestFrameworkToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.Framework
		expected pb.Framework
	}{
		{agentv1alpha1.FrameworkPydanticAI, pb.Framework_FRAMEWORK_PYDANTIC_AI},
		{agentv1alpha1.FrameworkLangChain, pb.Framework_FRAMEWORK_LANGCHAIN},
		{agentv1alpha1.FrameworkADK, pb.Framework_FRAMEWORK_GOOGLE_ADK},
		{agentv1alpha1.FrameworkMarvin, pb.Framework_FRAMEWORK_MARVIN},
		{agentv1alpha1.FrameworkAutogen, pb.Framework_FRAMEWORK_AUTOGEN},
		{agentv1alpha1.FrameworkA2A, pb.Framework_FRAMEWORK_A2A},
		{agentv1alpha1.Framework("unknown"), pb.Framework_FRAMEWORK_UNSPECIFIED},
		{agentv1alpha1.Framework(""), pb.Framework_FRAMEWORK_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := FrameworkToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("FrameworkToProto(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
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

func TestRuntimeTypeToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.RuntimeType
		expected pb.RuntimeType
	}{
		{agentv1alpha1.RuntimeTypeStandard, pb.RuntimeType_RUNTIME_TYPE_STANDARD},
		{agentv1alpha1.RuntimeType("unknown"), pb.RuntimeType_RUNTIME_TYPE_UNSPECIFIED},
		{agentv1alpha1.RuntimeType(""), pb.RuntimeType_RUNTIME_TYPE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := RuntimeTypeToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("RuntimeTypeToProto(%q) = %v, want %v", tt.input, result, tt.expected)
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
