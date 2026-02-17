package builder

import (
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func int32Ptr(i int32) *int32 { return &i }

func TestBuildScaledObject(t *testing.T) {
	t.Run("basic ScaledObject with all fields", func(t *testing.T) {
		params := ScaledObjectParams{
			AgentName:      "my-agent",
			AgentNamespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name": "my-agent",
				"flokoa.ai/agent":        "my-agent",
			},
			DeploymentName: "my-agent",
			Scaling: agentv1alpha1.ScalingSpec{
				MinReplicaCount: int32Ptr(0),
				MaxReplicaCount: int32Ptr(10),
				CooldownPeriod:  int32Ptr(300),
				PollingInterval: int32Ptr(30),
				Triggers: []agentv1alpha1.ScalingTrigger{
					{
						Type: "prometheus",
						Metadata: map[string]string{
							"serverAddress": "http://prometheus:9090",
							"threshold":     "100",
							"query":         "sum(rate(http_requests_total[2m]))",
						},
					},
				},
			},
		}

		obj := BuildScaledObject(params)

		// Verify GVK
		if obj.GetKind() != "ScaledObject" {
			t.Errorf("kind = %q, want ScaledObject", obj.GetKind())
		}
		if obj.GetAPIVersion() != "keda.sh/v1alpha1" {
			t.Errorf("apiVersion = %q, want keda.sh/v1alpha1", obj.GetAPIVersion())
		}

		// Verify name
		if obj.GetName() != "my-agent-scaler" {
			t.Errorf("name = %q, want my-agent-scaler", obj.GetName())
		}
		if obj.GetNamespace() != "default" {
			t.Errorf("namespace = %q, want default", obj.GetNamespace())
		}

		// Verify labels
		labels := obj.GetLabels()
		if labels["flokoa.ai/agent"] != "my-agent" {
			t.Errorf("label flokoa.ai/agent = %q, want my-agent", labels["flokoa.ai/agent"])
		}

		// Verify spec fields
		spec, ok := obj.Object["spec"].(map[string]interface{})
		if !ok {
			t.Fatal("spec is not a map")
		}

		if minReplicas, ok := spec["minReplicaCount"].(int64); !ok || minReplicas != 0 {
			t.Errorf("minReplicaCount = %v, want 0", spec["minReplicaCount"])
		}
		if maxReplicas, ok := spec["maxReplicaCount"].(int64); !ok || maxReplicas != 10 {
			t.Errorf("maxReplicaCount = %v, want 10", spec["maxReplicaCount"])
		}
		if cooldown, ok := spec["cooldownPeriod"].(int64); !ok || cooldown != 300 {
			t.Errorf("cooldownPeriod = %v, want 300", spec["cooldownPeriod"])
		}
		if polling, ok := spec["pollingInterval"].(int64); !ok || polling != 30 {
			t.Errorf("pollingInterval = %v, want 30", spec["pollingInterval"])
		}

		// Verify scaleTargetRef
		targetRef, ok := spec["scaleTargetRef"].(map[string]interface{})
		if !ok {
			t.Fatal("scaleTargetRef is not a map")
		}
		if targetRef["name"] != "my-agent" {
			t.Errorf("scaleTargetRef.name = %q, want my-agent", targetRef["name"])
		}
		if targetRef["kind"] != "Deployment" {
			t.Errorf("scaleTargetRef.kind = %q, want Deployment", targetRef["kind"])
		}

		// Verify triggers
		triggers, ok := spec["triggers"].([]interface{})
		if !ok {
			t.Fatal("triggers is not a slice")
		}
		if len(triggers) != 1 {
			t.Fatalf("len(triggers) = %d, want 1", len(triggers))
		}

		trigger, ok := triggers[0].(map[string]interface{})
		if !ok {
			t.Fatal("trigger is not a map")
		}
		if trigger["type"] != "prometheus" {
			t.Errorf("trigger type = %q, want prometheus", trigger["type"])
		}

		metadata, ok := trigger["metadata"].(map[string]interface{})
		if !ok {
			t.Fatal("trigger metadata is not a map")
		}
		if metadata["serverAddress"] != "http://prometheus:9090" {
			t.Errorf("metadata.serverAddress = %q, want http://prometheus:9090", metadata["serverAddress"])
		}
	})

	t.Run("trigger with authenticationRef", func(t *testing.T) {
		params := ScaledObjectParams{
			AgentName:      "auth-agent",
			AgentNamespace: "prod",
			Labels:         map[string]string{},
			DeploymentName: "auth-agent",
			Scaling: agentv1alpha1.ScalingSpec{
				Triggers: []agentv1alpha1.ScalingTrigger{
					{
						Type:     "prometheus",
						Metadata: map[string]string{"threshold": "50"},
						AuthenticationRef: &agentv1alpha1.ScalingTriggerAuth{
							Name: "prom-auth",
							Kind: "TriggerAuthentication",
						},
						MetricType: "AverageValue",
					},
				},
			},
		}

		obj := BuildScaledObject(params)
		spec := obj.Object["spec"].(map[string]interface{})
		triggers := spec["triggers"].([]interface{})
		trigger := triggers[0].(map[string]interface{})

		authRef, ok := trigger["authenticationRef"].(map[string]interface{})
		if !ok {
			t.Fatal("authenticationRef is not a map")
		}
		if authRef["name"] != "prom-auth" {
			t.Errorf("authRef.name = %q, want prom-auth", authRef["name"])
		}
		if authRef["kind"] != "TriggerAuthentication" {
			t.Errorf("authRef.kind = %q, want TriggerAuthentication", authRef["kind"])
		}
		if trigger["metricType"] != "AverageValue" {
			t.Errorf("metricType = %q, want AverageValue", trigger["metricType"])
		}
	})

	t.Run("trigger with name", func(t *testing.T) {
		params := ScaledObjectParams{
			AgentName:      "named-trigger",
			AgentNamespace: "default",
			Labels:         map[string]string{},
			DeploymentName: "named-trigger",
			Scaling: agentv1alpha1.ScalingSpec{
				Triggers: []agentv1alpha1.ScalingTrigger{
					{
						Type:     "cron",
						Name:     "business-hours",
						Metadata: map[string]string{"timezone": "UTC", "start": "0 8 * * *", "end": "0 18 * * *", "desiredReplicas": "3"},
					},
				},
			},
		}

		obj := BuildScaledObject(params)
		spec := obj.Object["spec"].(map[string]interface{})
		triggers := spec["triggers"].([]interface{})
		trigger := triggers[0].(map[string]interface{})

		if trigger["name"] != "business-hours" {
			t.Errorf("trigger name = %q, want business-hours", trigger["name"])
		}
	})

	t.Run("omits optional fields when nil", func(t *testing.T) {
		params := ScaledObjectParams{
			AgentName:      "minimal",
			AgentNamespace: "default",
			Labels:         map[string]string{},
			DeploymentName: "minimal",
			Scaling: agentv1alpha1.ScalingSpec{
				Triggers: []agentv1alpha1.ScalingTrigger{
					{
						Type:     "cpu",
						Metadata: map[string]string{"type": "Utilization", "value": "50"},
					},
				},
			},
		}

		obj := BuildScaledObject(params)
		spec := obj.Object["spec"].(map[string]interface{})

		if _, ok := spec["minReplicaCount"]; ok {
			t.Error("minReplicaCount should not be set when nil")
		}
		if _, ok := spec["maxReplicaCount"]; ok {
			t.Error("maxReplicaCount should not be set when nil")
		}
		if _, ok := spec["cooldownPeriod"]; ok {
			t.Error("cooldownPeriod should not be set when nil")
		}
		if _, ok := spec["pollingInterval"]; ok {
			t.Error("pollingInterval should not be set when nil")
		}
	})

	t.Run("multiple triggers", func(t *testing.T) {
		params := ScaledObjectParams{
			AgentName:      "multi",
			AgentNamespace: "default",
			Labels:         map[string]string{},
			DeploymentName: "multi",
			Scaling: agentv1alpha1.ScalingSpec{
				MinReplicaCount: int32Ptr(1),
				MaxReplicaCount: int32Ptr(20),
				Triggers: []agentv1alpha1.ScalingTrigger{
					{Type: "prometheus", Metadata: map[string]string{"threshold": "100"}},
					{Type: "cpu", Metadata: map[string]string{"type": "Utilization", "value": "80"}},
				},
			},
		}

		obj := BuildScaledObject(params)
		spec := obj.Object["spec"].(map[string]interface{})
		triggers := spec["triggers"].([]interface{})

		if len(triggers) != 2 {
			t.Fatalf("len(triggers) = %d, want 2", len(triggers))
		}

		t0 := triggers[0].(map[string]interface{})
		t1 := triggers[1].(map[string]interface{})
		if t0["type"] != "prometheus" {
			t.Errorf("triggers[0].type = %q, want prometheus", t0["type"])
		}
		if t1["type"] != "cpu" {
			t.Errorf("triggers[1].type = %q, want cpu", t1["type"])
		}
	})
}

func TestScaledObjectName(t *testing.T) {
	if got := ScaledObjectName("my-agent"); got != "my-agent-scaler" {
		t.Errorf("ScaledObjectName(my-agent) = %q, want my-agent-scaler", got)
	}
}
