package converter

import (
	"encoding/json"

	"google.golang.org/protobuf/types/known/structpb"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

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
		Card:    AgentCardToProto(&spec.Card),
		Runtime: AgentRuntimeToProto(&spec.Runtime),
		Spec:    fragmentToStruct(spec.Spec),
	}

	if spec.ModelRef != nil {
		pbSpec.ModelRef = NamespacedRefToProto(spec.ModelRef)
	}
	for i := range spec.InstructionRefs {
		pbSpec.InstructionRefs = append(pbSpec.InstructionRefs, NamespacedRefToProto(&spec.InstructionRefs[i]))
	}
	for i := range spec.Tools {
		pbSpec.Tools = append(pbSpec.Tools, NamespacedRefToProto(&spec.Tools[i]))
	}
	for i := range spec.Capabilities {
		att := &spec.Capabilities[i]
		pbSpec.Capabilities = append(pbSpec.Capabilities, &pb.CapabilityAttachment{
			Ref:    NamespacedRefToProto(&att.Ref),
			Config: jsonToStruct(att.Config),
		})
	}
	if len(spec.SecretRefs) > 0 {
		pbSpec.SecretRefs = map[string]*pb.SecretKeySelector{}
		for name, sel := range spec.SecretRefs {
			pbSpec.SecretRefs[name] = &pb.SecretKeySelector{
				Name:     sel.Name,
				Key:      sel.Key,
				Optional: sel.Optional != nil && *sel.Optional,
			}
		}
	}

	return pbSpec
}

// NamespacedRefToProto converts a NamespacedRef to proto.
func NamespacedRefToProto(ref *agentv1alpha1.NamespacedRef) *pb.NamespacedRef {
	if ref == nil {
		return nil
	}
	return &pb.NamespacedRef{Name: ref.Name, Namespace: ref.Namespace}
}

// fragmentToStruct renders the inline AgentSpec fragment in its JSON form.
func fragmentToStruct(frag *agentv1alpha1.AgentSpecFragment) *structpb.Struct {
	if frag == nil {
		return nil
	}
	raw, err := json.Marshal(frag)
	if err != nil {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}
	s, err := structpb.NewStruct(data)
	if err != nil {
		return nil
	}
	return s
}

// jsonToStruct converts an apiextensions JSON object to a proto Struct.
func jsonToStruct(j *apiextensionsv1.JSON) *structpb.Struct {
	if j == nil || len(j.Raw) == 0 {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(j.Raw, &data); err != nil {
		return nil
	}
	s, err := structpb.NewStruct(data)
	if err != nil {
		return nil
	}
	return s
}

// AgentCardToProto converts AgentCard to proto.
func AgentCardToProto(card *agentv1alpha1.AgentCardOverride) *pb.AgentCard {
	if card == nil {
		return nil
	}

	pbCard := &pb.AgentCard{
		Name:        card.Name,
		Description: card.Description,
		Version:     card.Version,
	}

	for _, mode := range card.DefaultInputModes {
		pbCard.DefaultInputModes = append(pbCard.DefaultInputModes, InputOutputModeToProto(mode))
	}
	for _, mode := range card.DefaultOutputModes {
		pbCard.DefaultOutputModes = append(pbCard.DefaultOutputModes, InputOutputModeToProto(mode))
	}

	pbCard.Capabilities = AgentCapabilitiesToProto(&card.Capabilities)

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

// AgentRuntimeToProto converts AgentRuntime to proto.
func AgentRuntimeToProto(runtime *agentv1alpha1.AgentRuntime) *pb.AgentRuntime {
	if runtime == nil {
		return nil
	}

	pbRuntime := &pb.AgentRuntime{
		Image:              runtime.Image,
		RunnerVersion:      runtime.RunnerVersion,
		Isolation:          IsolationTierToProto(runtime.Isolation),
		ServiceAccountName: runtime.ServiceAccountName,
		NodeSelector:       runtime.NodeSelector,
	}
	if runtime.Replicas != nil {
		pbRuntime.Replicas = *runtime.Replicas
	}
	for _, env := range runtime.Env {
		pbRuntime.Env = append(pbRuntime.Env, &pb.EnvVar{Name: env.Name, Value: env.Value})
	}
	if runtime.Resources != nil {
		pbRuntime.Resources = &pb.ResourceRequirements{
			Limits:   map[string]string{},
			Requests: map[string]string{},
		}
		for name, qty := range runtime.Resources.Limits {
			pbRuntime.Resources.Limits[string(name)] = qty.String()
		}
		for name, qty := range runtime.Resources.Requests {
			pbRuntime.Resources.Requests[string(name)] = qty.String()
		}
	}
	for _, ref := range runtime.ImagePullSecrets {
		pbRuntime.ImagePullSecrets = append(pbRuntime.ImagePullSecrets, &pb.LocalObjectReference{Name: ref.Name})
	}

	return pbRuntime
}

// AgentStatusToProto converts AgentStatus to proto.
func AgentStatusToProto(status *agentv1alpha1.AgentStatus) *pb.AgentStatus {
	if status == nil {
		return nil
	}

	return &pb.AgentStatus{
		Phase:                AgentPhaseToProto(status.Phase),
		Url:                  status.URL,
		SpecHash:             status.SpecHash,
		RunnerVersion:        status.RunnerVersion,
		InjectedCapabilities: status.InjectedCapabilities,
		Replicas:             status.Replicas,
		AvailableReplicas:    status.AvailableReplicas,
		Conditions:           ConditionsToProto(status.Conditions),
		ObservedGeneration:   status.ObservedGeneration,
	}
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

// IsolationTierToProto converts IsolationTier enum to proto.
func IsolationTierToProto(tier agentv1alpha1.IsolationTier) pb.IsolationTier {
	switch tier {
	case agentv1alpha1.IsolationShared:
		return pb.IsolationTier_ISOLATION_TIER_SHARED
	case agentv1alpha1.IsolationSession:
		return pb.IsolationTier_ISOLATION_TIER_SESSION
	default:
		return pb.IsolationTier_ISOLATION_TIER_UNSPECIFIED
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
