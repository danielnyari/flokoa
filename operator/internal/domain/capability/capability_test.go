package capability

import (
	"strings"
	"testing"
)

func runner() RunnerInfo {
	return RunnerInfo{
		RunnerVersion: "0.2.0",
		Python:        "3.13",
		PydanticAI:    "1.107.0",
		Baseline: map[string]string{
			"httpx":    "0.28.1",
			"pydantic": "2.12.5",
		},
	}
}

func TestCheckRequires(t *testing.T) {
	tests := []struct {
		name    string
		req     Requires
		wantErr string
	}{
		{
			name: "compatible tuple",
			req:  Requires{Python: "3.13", PydanticAI: ">=1.100,<2", FlokoaRunner: ">=0.2"},
		},
		{
			name: "empty fields are skipped",
			req:  Requires{},
		},
		{
			name:    "python minor mismatch",
			req:     Requires{Python: "3.12"},
			wantErr: `capability "kb-tools" requires python "3.12" but runner 0.2.0 provides python "3.13"`,
		},
		{
			name:    "pydantic-ai incompatible",
			req:     Requires{PydanticAI: ">=2,<3"},
			wantErr: `capability "kb-tools" requires pydantic-ai ">=2,<3" but runner 0.2.0 provides pydantic-ai "1.107.0"`,
		},
		{
			name:    "flokoa-runner incompatible",
			req:     Requires{FlokoaRunner: ">=0.3"},
			wantErr: `capability "kb-tools" requires flokoa-runner ">=0.3" but runner 0.2.0 provides flokoa-runner "0.2.0"`,
		},
		{
			name:    "invalid specifier",
			req:     Requires{PydanticAI: "not-a-specifier"},
			wantErr: `capability "kb-tools" declares an invalid pydantic-ai specifier "not-a-specifier"`,
		},
		{
			name: "exact pin specifier",
			req:  Requires{PydanticAI: "==1.107.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckRequires("kb-tools", tt.req, runner())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("CheckRequires() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("CheckRequires() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CheckRequires() = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNormalizePackageName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"pydantic-ai-harness", "pydantic-ai-harness"},
		{"Pydantic_AI.Harness", "pydantic-ai-harness"},
		{"foo__bar--baz..qux", "foo-bar-baz-qux"},
		{"UPPER", "upper"},
	}
	for _, tt := range tests {
		if got := NormalizePackageName(tt.in); got != tt.want {
			t.Errorf("NormalizePackageName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParsePin(t *testing.T) {
	tests := []struct {
		pin         string
		wantName    string
		wantVersion string
		wantErr     bool
	}{
		{pin: "pydantic-ai-harness==0.2.1", wantName: "pydantic-ai-harness", wantVersion: "0.2.1"},
		{pin: "Foo_Bar==1.0", wantName: "foo-bar", wantVersion: "1.0"},
		{pin: "foo>=1.0", wantErr: true},
		{pin: "foo", wantErr: true},
		{pin: "foo==1.0; python_version<'3.14'", wantErr: true},
		{pin: "==1.0", wantErr: true},
		{pin: "foo==", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.pin, func(t *testing.T) {
			name, version, err := ParsePin(tt.pin)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePin(%q) = (%q, %q, nil), want error", tt.pin, name, version)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePin(%q) error: %v", tt.pin, err)
			}
			if name != tt.wantName || version != tt.wantVersion {
				t.Fatalf("ParsePin(%q) = (%q, %q), want (%q, %q)", tt.pin, name, version, tt.wantName, tt.wantVersion)
			}
		})
	}
}

func TestDetectConflicts(t *testing.T) {
	tests := []struct {
		name string
		caps []Deps
		want []string
	}{
		{
			name: "disjoint pins",
			caps: []Deps{
				{Name: "a", Pins: []string{"left-pad==1.0.0"}},
				{Name: "b", Pins: []string{"right-pad==2.0.0"}},
			},
		},
		{
			name: "two capabilities pin different harness versions (the canonical rejection)",
			caps: []Deps{
				{Name: "a", Pins: []string{"pydantic-ai-harness==0.2.1"}},
				{Name: "b", Pins: []string{"pydantic-ai-harness==0.3.0"}},
			},
			want: []string{
				`capabilities "a" and "b" pin conflicting versions of pydantic-ai-harness (0.2.1 vs 0.3.0)`,
			},
		},
		{
			name: "same package same version is fine",
			caps: []Deps{
				{Name: "a", Pins: []string{"shared==1.2.3"}},
				{Name: "b", Pins: []string{"shared==1.2.3"}},
			},
		},
		{
			name: "pin collides with the runner baseline",
			caps: []Deps{
				{Name: "a", Pins: []string{"httpx==0.20.0"}},
			},
			want: []string{
				`capability "a" pins httpx==0.20.0 but the runner 0.2.0 baseline pins httpx==0.28.1`,
			},
		},
		{
			name: "pin matching the baseline version is fine",
			caps: []Deps{
				{Name: "a", Pins: []string{"httpx==0.28.1"}},
			},
		},
		{
			name: "normalized names collide",
			caps: []Deps{
				{Name: "a", Pins: []string{"Pydantic_AI_Harness==0.2.1"}},
				{Name: "b", Pins: []string{"pydantic-ai-harness==0.3.0"}},
			},
			want: []string{
				`capabilities "a" and "b" pin conflicting versions of pydantic-ai-harness (0.2.1 vs 0.3.0)`,
			},
		},
		{
			name: "invalid pin is reported",
			caps: []Deps{
				{Name: "a", Pins: []string{"not-a-pin"}},
			},
			want: []string{
				`capability "a" has an invalid dependency pin "not-a-pin": must be name==version`,
			},
		},
		{
			name: "multiple conflicts all reported in order",
			caps: []Deps{
				{Name: "a", Pins: []string{"pkg==1.0", "httpx==0.20.0"}},
				{Name: "b", Pins: []string{"pkg==2.0"}},
			},
			want: []string{
				`capability "a" pins httpx==0.20.0 but the runner 0.2.0 baseline pins httpx==0.28.1`,
				`capabilities "a" and "b" pin conflicting versions of pkg (1.0 vs 2.0)`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectConflicts(tt.caps, runner())
			if len(got) != len(tt.want) {
				t.Fatalf("DetectConflicts() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("DetectConflicts()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateRequires(t *testing.T) {
	tests := []struct {
		name    string
		req     Requires
		wantErr string
	}{
		{name: "all empty", req: Requires{}},
		{name: "valid specifiers", req: Requires{Python: "3.13", PydanticAI: ">=1.100,<2", FlokoaRunner: ">=0.2"}},
		{
			name:    "invalid pydantic-ai specifier",
			req:     Requires{PydanticAI: "not-a-specifier"},
			wantErr: `invalid pydantic-ai specifier "not-a-specifier"`,
		},
		{
			name:    "invalid flokoa-runner specifier",
			req:     Requires{FlokoaRunner: "~~1"},
			wantErr: `invalid flokoa-runner specifier "~~1"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequires(tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateRequires() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateRequires() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEntryName(t *testing.T) {
	tests := []struct {
		entrypoint string
		override   string
		want       string
	}{
		{"flokoa_openapi.capability:OpenAPI", "", "OpenAPI"},
		{"flokoa_openapi.capability:OpenAPI", "flokoa.OpenAPI", "flokoa.OpenAPI"},
		{"pkg.mod:Outer.Inner", "", "Outer.Inner"},
	}
	for _, tt := range tests {
		if got := EntryName(tt.entrypoint, tt.override); got != tt.want {
			t.Errorf("EntryName(%q, %q) = %q, want %q", tt.entrypoint, tt.override, got, tt.want)
		}
	}
}

func TestCompileSchema(t *testing.T) {
	tests := []struct {
		name    string
		schema  string
		wantErr bool
	}{
		{name: "valid object schema", schema: `{"type":"object","properties":{"x":{"type":"string"}}}`},
		{name: "empty object is a valid schema", schema: `{}`},
		{name: "invalid type keyword", schema: `{"type":12}`},
		{name: "not json", schema: `{`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CompileSchema([]byte(tt.schema))
			if tt.name == "invalid type keyword" {
				// jsonschema must reject a numeric `type`.
				if err == nil {
					t.Fatal("CompileSchema() = nil, want error for invalid schema")
				}
				return
			}
			if tt.wantErr && err == nil {
				t.Fatal("CompileSchema() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("CompileSchema() = %v, want nil", err)
			}
		})
	}
}

const kbSchema = `{
	"type": "object",
	"required": ["endpoint"],
	"properties": {
		"endpoint": {"type": "string", "pattern": "^https://"},
		"apiKey": {"type": "string", "minLength": 20},
		"mode": {"type": "string", "enum": ["read", "write"]},
		"maxResults": {"type": "integer"},
		"nested": {
			"type": "object",
			"properties": {"token": {"type": "string", "pattern": "^tok-"}}
		}
	},
	"additionalProperties": false
}`

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		wantPath string
		wantOK   bool
	}{
		{
			name:   "valid config",
			config: `{"endpoint": "https://kb.example.com", "mode": "read", "maxResults": 5}`,
			wantOK: true,
		},
		{
			name:     "missing required key",
			config:   `{"mode": "read"}`,
			wantPath: "",
			wantOK:   false,
		},
		{
			name:     "wrong type",
			config:   `{"endpoint": "https://kb.example.com", "maxResults": "five"}`,
			wantPath: "/maxResults",
			wantOK:   false,
		},
		{
			name:     "unknown property",
			config:   `{"endpoint": "https://kb.example.com", "surprise": true}`,
			wantPath: "",
			wantOK:   false,
		},
		{
			name:   "placeholder satisfies pattern",
			config: `{"endpoint": "${secret:kb-endpoint}"}`,
			wantOK: true,
		},
		{
			name:   "placeholder satisfies minLength",
			config: `{"endpoint": "https://kb.example.com", "apiKey": "${secret:kb-key}"}`,
			wantOK: true,
		},
		{
			name:   "placeholder satisfies enum",
			config: `{"endpoint": "https://kb.example.com", "mode": "${secret:kb-mode}"}`,
			wantOK: true,
		},
		{
			name:   "placeholder in nested object satisfies pattern",
			config: `{"endpoint": "https://kb.example.com", "nested": {"token": "${secret:kb-token}"}}`,
			wantOK: true,
		},
		{
			name:     "placeholder does not satisfy a non-string type",
			config:   `{"endpoint": "https://kb.example.com", "maxResults": "${secret:kb-max}"}`,
			wantPath: "/maxResults",
			wantOK:   false,
		},
		{
			name:     "plain string still fails pattern",
			config:   `{"endpoint": "http://insecure.example.com"}`,
			wantPath: "/endpoint",
			wantOK:   false,
		},
		{
			name:     "config must be valid json",
			config:   `{`,
			wantOK:   false,
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig([]byte(kbSchema), []byte(tt.config))
			if tt.wantOK {
				if err != nil {
					t.Fatalf("ValidateConfig() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ValidateConfig() = nil, want error")
			}
			var cfgErr *ConfigError
			if ok := errorAs(err, &cfgErr); ok && tt.wantPath != "" && cfgErr.Path != tt.wantPath {
				t.Fatalf("ValidateConfig() path = %q, want %q (err: %v)", cfgErr.Path, tt.wantPath, err)
			}
		})
	}
}

func errorAs(err error, target **ConfigError) bool {
	v, ok := err.(*ConfigError)
	if ok {
		*target = v
	}
	return ok
}
