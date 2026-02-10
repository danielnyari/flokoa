/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// minimalCard creates a minimal valid AgentCard for testing
func minimalCard() agentv1alpha1.AgentCardOverride {
	return agentv1alpha1.AgentCardOverride{
		Name:        "Test Agent",
		Description: "A test agent",
		Version:     "1.0.0",
		Skills: []agentv1alpha1.AgentSkill{
			{
				ID:          "test-skill",
				Name:        "Test Skill",
				Description: "A test skill",
				Tags:        []string{"test"},
			},
		},
	}
}
