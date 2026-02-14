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

package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

func TestValidateAgentTool(t *testing.T) {
	tests := []struct {
		name    string
		obj     *AgentTool
		wantErr bool
	}{
		{
			name: "valid openapi tool with URL and inline schema",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						URL: "https://api.example.com",
						OpenApiSchema: OpenApiSchema{
							Value: &runtime.RawExtension{Raw: []byte(`{"openapi":"3.0.0"}`)},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid openapi tool with serviceRef and endpointPath",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						ServiceRef: &ServiceRef{
							Name: "my-service",
							Port: ptr.To[int32](8080),
						},
						OpenApiSchema: OpenApiSchema{
							EndpointPath: "/openapi.json",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid openapi tool with serviceRef using portName",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						ServiceRef: &ServiceRef{
							Name:     "my-service",
							PortName: "http",
						},
						OpenApiSchema: OpenApiSchema{
							EndpointPath: "/openapi.json",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "openapi type without openApi config",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
				},
			},
			wantErr: true,
		},
		{
			name: "both url and serviceRef set",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						URL: "https://api.example.com",
						ServiceRef: &ServiceRef{
							Name: "my-service",
							Port: ptr.To[int32](8080),
						},
						OpenApiSchema: OpenApiSchema{
							EndpointPath: "/openapi.json",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "neither url nor serviceRef set",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						OpenApiSchema: OpenApiSchema{
							EndpointPath: "/openapi.json",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no schema source set",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						URL:           "https://api.example.com",
						OpenApiSchema: OpenApiSchema{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple schema sources set",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						URL: "https://api.example.com",
						OpenApiSchema: OpenApiSchema{
							Value:        &runtime.RawExtension{Raw: []byte(`{}`)},
							EndpointPath: "/openapi.json",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "serviceRef with both port and portName",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						ServiceRef: &ServiceRef{
							Name:     "my-service",
							Port:     ptr.To[int32](8080),
							PortName: "http",
						},
						OpenApiSchema: OpenApiSchema{
							EndpointPath: "/openapi.json",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "serviceRef with neither port nor portName",
			obj: &AgentTool{
				Spec: AgentToolSpec{
					Type:        AgentToolTypeOpenAPI,
					Description: "A test tool",
					OpenApi: &OpenApiToolSpec{
						ServiceRef: &ServiceRef{
							Name: "my-service",
						},
						OpenApiSchema: OpenApiSchema{
							EndpointPath: "/openapi.json",
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgentTool(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAgentTool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
