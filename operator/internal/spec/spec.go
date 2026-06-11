// Package spec embeds the AgentSpec JSON Schemas published by runner releases
// (runtime contract, docs/reference/runtime-contract.md) and validates compiled
// agent specs against them. No Python in the control plane: the schemas are
// generated inside the runner environment (sdk/python: make runner-contract)
// and committed under schemas/.
package spec

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var errPrinter = message.NewPrinter(language.English)

//go:embed schemas/*.json
var schemaFS embed.FS

// DefaultRunnerVersion is the runner release the operator pairs with by
// default. Agents may pin another supported version via
// spec.runtime.runnerVersion (one schema per supported runner version is
// embedded; the support window is ~2 concurrent versions).
var DefaultRunnerVersion = "0.2.0"

var (
	mu       sync.Mutex
	compiled = map[string]*jsonschema.Schema{}
	digests  = map[string]string{}
)

func schemaPath(runnerVersion string) string {
	return fmt.Sprintf("schemas/agentspec-%s.json", runnerVersion)
}

// SupportedRunnerVersions lists the runner versions with an embedded schema.
func SupportedRunnerVersions() []string {
	entries, err := schemaFS.ReadDir("schemas")
	if err != nil {
		return nil
	}
	versions := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if v, ok := strings.CutPrefix(name, "agentspec-"); ok {
			versions = append(versions, strings.TrimSuffix(v, ".json"))
		}
	}
	sort.Strings(versions)
	return versions
}

// LoadSchema compiles (and caches) the embedded AgentSpec schema for a runner
// version. An unknown version is how runner-version skew surfaces: callers
// turn it into a SpecValid=False condition rather than deploying a pod the
// runner would reject.
func LoadSchema(runnerVersion string) (*jsonschema.Schema, error) {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := compiled[runnerVersion]; ok {
		return s, nil
	}
	raw, err := schemaFS.ReadFile(schemaPath(runnerVersion))
	if err != nil {
		return nil, fmt.Errorf(
			"no embedded AgentSpec schema for runner version %q (supported: %s): %w",
			runnerVersion, strings.Join(SupportedRunnerVersions(), ", "), err,
		)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing embedded schema for runner %s: %w", runnerVersion, err)
	}
	c := jsonschema.NewCompiler()
	url := "flokoa://" + schemaPath(runnerVersion)
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("loading embedded schema for runner %s: %w", runnerVersion, err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compiling embedded schema for runner %s: %w", runnerVersion, err)
	}
	compiled[runnerVersion] = s
	digests[runnerVersion] = fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	return s, nil
}

// SchemaDigest returns the sha256 digest of the embedded schema for a runner
// version, in the same format the runner manifest records
// (agentSpecSchemaDigest). The runner cross-checks this at bootstrap so
// operator↔image skew fails loudly.
func SchemaDigest(runnerVersion string) (string, error) {
	if _, err := LoadSchema(runnerVersion); err != nil {
		return "", err
	}
	mu.Lock()
	defer mu.Unlock()
	return digests[runnerVersion], nil
}

// ValidationError describes why a compiled spec failed schema validation,
// with a JSON-pointer-style path to the offending value.
type ValidationError struct {
	// Path is the instance location, e.g. "/capabilities/1/MCP/url".
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s at %s", e.Message, e.Path)
}

// Validate checks a compiled AgentSpec document against the embedded schema
// for the given runner version. doc must be the JSON-generic form
// (map[string]any with json-compatible leaves).
func Validate(runnerVersion string, doc any) error {
	s, err := LoadSchema(runnerVersion)
	if err != nil {
		return err
	}
	if err := s.Validate(doc); err != nil {
		var verr *jsonschema.ValidationError
		if ok := asValidationError(err, &verr); ok {
			leaf := leafCause(verr)
			return &ValidationError{
				Path:    "/" + strings.Join(leaf.InstanceLocation, "/"),
				Message: leaf.ErrorKind.LocalizedString(errPrinter),
			}
		}
		return err
	}
	return nil
}

func asValidationError(err error, target **jsonschema.ValidationError) bool {
	v, ok := err.(*jsonschema.ValidationError)
	if ok {
		*target = v
	}
	return ok
}

// leafCause walks to the deepest single cause for a concise error path. When
// a node fans out (anyOf alternatives), the node itself is the most honest
// report; descending one branch arbitrarily would mislead.
func leafCause(v *jsonschema.ValidationError) *jsonschema.ValidationError {
	for len(v.Causes) == 1 {
		v = v.Causes[0]
	}
	if len(v.Causes) > 1 {
		// Prefer the cause whose instance path is deepest — typically the
		// alternative the user actually attempted.
		deepest := v
		depth := -1
		for _, c := range v.Causes {
			leaf := leafCause(c)
			if d := len(leaf.InstanceLocation); d > depth {
				deepest, depth = leaf, d
			}
		}
		return deepest
	}
	return v
}
