package converter

import (
	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// AgentToProto converts a Kubernetes Agent to proto.
func AgentToProto(agent *agentv1alpha1.Agent) *pb.Agent {
	if agent == nil {
		return nil
	}

	return &pb.Agent{
		Metadata: ObjectMetaToProto(&agent.ObjectMeta),
		Spec:     AgentSpecToProto(&agent.Spec),
		Status:   AgentStatusToProto(&agent.Status),
	}
}

// AgentSpecToProto converts AgentSpec to proto.
func AgentSpecToProto(spec *agentv1alpha1.AgentSpec) *pb.AgentSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.AgentSpec{
		Framework: FrameworkToProto(spec.Framework),
	}

	// Convert Card
	pbSpec.Card = AgentCardToProto(&spec.Card)

	// Convert Runtime
	pbSpec.Runtime = RuntimeSpecToProto(&spec.Runtime)

	// Convert Model reference
	if spec.Model != nil {
		pbSpec.Model = &pb.AgentModelRef{
			Name:      spec.Model.Name,
			Namespace: spec.Model.Namespace,
		}
	}

	// Convert Tools
	for _, tool := range spec.Tools {
		pbSpec.Tools = append(pbSpec.Tools, ToolEntryToProto(&tool))
	}

	return pbSpec
}

// AgentCardToProto converts AgentCard to proto.
func AgentCardToProto(card *agentv1alpha1.AgentCard) *pb.AgentCard {
	if card == nil {
		return nil
	}

	pbCard := &pb.AgentCard{
		Name:        card.Name,
		Description: card.Description,
		Version:     card.Version,
	}

	// Convert input/output modes
	for _, mode := range card.DefaultInputModes {
		pbCard.DefaultInputModes = append(pbCard.DefaultInputModes, InputOutputModeToProto(mode))
	}
	for _, mode := range card.DefaultOutputModes {
		pbCard.DefaultOutputModes = append(pbCard.DefaultOutputModes, InputOutputModeToProto(mode))
	}

	// Convert capabilities
	pbCard.Capabilities = AgentCapabilitiesToProto(&card.Capabilities)

	// Convert skills
	for _, skill := range card.Skills {
		pbCard.Skills = append(pbCard.Skills, AgentSkillToProto(&skill))
	}

	return pbCard
}

// AgentCapabilitiesToProto converts AgentCapabilities to proto.
func AgentCapabilitiesToProto(caps *agentv1alpha1.AgentCapabilities) *pb.AgentCapabilities {
	if caps == nil {
		return nil
	}

	return &pb.AgentCapabilities{
		PushNotifications:      caps.PushNotifications,
		StateTransitionHistory: caps.StateTransitionHistory,
		Streaming:              caps.Streaming,
	}
}

// AgentSkillToProto converts AgentSkill to proto.
func AgentSkillToProto(skill *agentv1alpha1.AgentSkill) *pb.AgentSkill {
	if skill == nil {
		return nil
	}

	return &pb.AgentSkill{
		Id:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		Tags:        skill.Tags,
		Examples:    skill.Examples,
		InputModes:  skill.InputModes,
		OutputModes: skill.OutputModes,
	}
}

// RuntimeSpecToProto converts RuntimeSpec to proto.
func RuntimeSpecToProto(runtime *agentv1alpha1.RuntimeSpec) *pb.RuntimeSpec {
	if runtime == nil {
		return nil
	}

	pbRuntime := &pb.RuntimeSpec{
		Type: RuntimeTypeToProto(runtime.Type),
	}

	if runtime.Spec != nil {
		pbRuntime.Spec = StandardRuntimeSpecToProto(runtime.Spec)
	}

	return pbRuntime
}

// StandardRuntimeSpecToProto converts StandardRuntimeSpec to proto.
func StandardRuntimeSpecToProto(spec *agentv1alpha1.StandardRuntimeSpec) *pb.StandardRuntimeSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.StandardRuntimeSpec{
		ServiceAccountName: spec.ServiceAccountName,
		NodeSelector:       spec.NodeSelector,
	}

	if spec.Replicas != nil {
		pbSpec.Replicas = *spec.Replicas
	}

	// Container conversion is complex - simplified for now
	pbSpec.Container = &pb.Container{
		Name:            spec.Container.Name,
		Image:           spec.Container.Image,
		Command:         spec.Container.Command,
		Args:            spec.Container.Args,
		WorkingDir:      spec.Container.WorkingDir,
		ImagePullPolicy: string(spec.Container.ImagePullPolicy),
	}

	return pbSpec
}

// ToolEntryToProto converts ToolEntry to proto.
func ToolEntryToProto(entry *agentv1alpha1.ToolEntry) *pb.ToolEntry {
	if entry == nil {
		return nil
	}

	pbEntry := &pb.ToolEntry{
		Name: entry.Name,
	}

	if entry.Inline != nil {
		pbEntry.Tool = &pb.ToolEntry_Inline{
			Inline: AgentToolSpecToProto(entry.Inline),
		}
	} else if entry.ToolRef != nil {
		pbEntry.Tool = &pb.ToolEntry_ToolRef{
			ToolRef: &pb.ToolRef{
				Name:      entry.ToolRef.Name,
				Namespace: entry.ToolRef.Namespace,
			},
		}
	}

	return pbEntry
}

// AgentStatusToProto converts AgentStatus to proto.
func AgentStatusToProto(status *agentv1alpha1.AgentStatus) *pb.AgentStatus {
	if status == nil {
		return nil
	}

	pbStatus := &pb.AgentStatus{
		Phase:              AgentPhaseToProto(status.Phase),
		Backend:            status.Backend,
		Url:                status.URL,
		Replicas:           status.Replicas,
		AvailableReplicas:  status.AvailableReplicas,
		DetectedFramework:  FrameworkToProto(status.DetectedFramework),
		Conditions:         ConditionsToProto(status.Conditions),
		ObservedGeneration: status.ObservedGeneration,
	}

	return pbStatus
}

// AgentListToProto converts a Kubernetes AgentList to proto.
func AgentListToProto(list *agentv1alpha1.AgentList) *pb.AgentList {
	if list == nil {
		return nil
	}

	pbList := &pb.AgentList{
		Metadata: ListMetaToProto(&list.ListMeta),
	}

	for i := range list.Items {
		pbList.Items = append(pbList.Items, AgentToProto(&list.Items[i]))
	}

	return pbList
}

// FrameworkToProto converts Framework enum to proto.
func FrameworkToProto(f agentv1alpha1.Framework) pb.Framework {
	switch f {
	case agentv1alpha1.FrameworkPydanticAI:
		return pb.Framework_FRAMEWORK_PYDANTIC_AI
	case agentv1alpha1.FrameworkLangChain:
		return pb.Framework_FRAMEWORK_LANGCHAIN
	case agentv1alpha1.FrameworkADK:
		return pb.Framework_FRAMEWORK_GOOGLE_ADK
	case agentv1alpha1.FrameworkMarvin:
		return pb.Framework_FRAMEWORK_MARVIN
	case agentv1alpha1.FrameworkAutogen:
		return pb.Framework_FRAMEWORK_AUTOGEN
	case agentv1alpha1.FrameworkA2A:
		return pb.Framework_FRAMEWORK_A2A
	default:
		return pb.Framework_FRAMEWORK_UNSPECIFIED
	}
}

// AgentPhaseToProto converts AgentPhase enum to proto.
func AgentPhaseToProto(phase agentv1alpha1.AgentPhase) pb.AgentPhase {
	switch phase {
	case agentv1alpha1.AgentPhasePending:
		return pb.AgentPhase_AGENT_PHASE_PENDING
	case agentv1alpha1.AgentPhaseRunning:
		return pb.AgentPhase_AGENT_PHASE_RUNNING
	case agentv1alpha1.AgentPhaseFailed:
		return pb.AgentPhase_AGENT_PHASE_FAILED
	default:
		return pb.AgentPhase_AGENT_PHASE_UNSPECIFIED
	}
}

// RuntimeTypeToProto converts RuntimeType enum to proto.
func RuntimeTypeToProto(rt agentv1alpha1.RuntimeType) pb.RuntimeType {
	switch rt {
	case agentv1alpha1.RuntimeTypeStandard:
		return pb.RuntimeType_RUNTIME_TYPE_STANDARD
	default:
		return pb.RuntimeType_RUNTIME_TYPE_UNSPECIFIED
	}
}

// InputOutputModeToProto converts InputOutputMode to proto.
func InputOutputModeToProto(mode agentv1alpha1.InputOutputMode) pb.InputOutputMode {
	switch mode {
	case agentv1alpha1.InputOutputModeText:
		return pb.InputOutputMode_INPUT_OUTPUT_MODE_TEXT
	case agentv1alpha1.InputOutputModeJSON:
		return pb.InputOutputMode_INPUT_OUTPUT_MODE_JSON
	default:
		return pb.InputOutputMode_INPUT_OUTPUT_MODE_UNSPECIFIED
	}
}
