package trigger

// Labels generates standard labels for child resources created by AgentTrigger.
func Labels(triggerName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       triggerName,
		"app.kubernetes.io/component":  "agenttrigger",
		"app.kubernetes.io/managed-by": "flokoa-operator",
		"flokoa.ai/trigger":            triggerName,
	}
}

// SensorName returns the deterministic name for the child Sensor.
func SensorName(triggerName string) string {
	return "at-" + triggerName
}

// ConfigMapName returns the deterministic name for the trigger config ConfigMap.
func ConfigMapName(triggerName string) string {
	return "agenttrigger-" + triggerName + "-config"
}
