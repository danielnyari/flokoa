// Package capability holds the pure domain logic for Capability admission
// (roadmap 08): requires-tuple evaluation against a runner release, pinned
// dependency-conflict detection across attachments and the runner baseline,
// and config validation against a capability's published JSON Schema with
// ${secret:NAME} placeholder tolerance (runtime contract §3, §5).
package capability

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	pep440 "github.com/aquasecurity/go-pep440-version"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var errPrinter = message.NewPrinter(language.English)

// RunnerInfo describes the runner release a capability is checked against
// (sourced from the operator-embedded runner baseline; runtime contract §5).
type RunnerInfo struct {
	// RunnerVersion is the runner release, e.g. "0.2.0".
	RunnerVersion string
	// Python is the exact Python minor, e.g. "3.13".
	Python string
	// PydanticAI is the pinned pydantic-ai version, e.g. "1.107.0".
	PydanticAI string
	// Baseline maps normalized package names to the versions pinned in the
	// runner's baseline lockfile (the full closure, not just headline libs).
	Baseline map[string]string
}

// Requires mirrors a capability artifact's compatibility tuple.
type Requires struct {
	// Python is an exact minor ("3.13"); empty skips the check.
	Python string
	// PydanticAI is a PEP 440 specifier set; empty skips the check.
	PydanticAI string
	// FlokoaRunner is a PEP 440 specifier set; empty skips the check.
	FlokoaRunner string
}

// Deps is one capability's pinned dependency closure for conflict detection.
type Deps struct {
	// Name is the Capability CR name (used in conflict messages).
	Name string
	// Pins are "name==version" entries mirrored from the artifact manifest.
	Pins []string
}

// CheckRequires evaluates a capability's requires tuple against a runner
// release. The returned error names both tuples (these messages are admission
// product surface).
func CheckRequires(name string, req Requires, runner RunnerInfo) error {
	if req.Python != "" && req.Python != runner.Python {
		return fmt.Errorf("capability %q requires python %q but runner %s provides python %q",
			name, req.Python, runner.RunnerVersion, runner.Python)
	}

	for _, c := range []struct{ key, specifier, provided string }{
		{"pydantic-ai", req.PydanticAI, runner.PydanticAI},
		{"flokoa-runner", req.FlokoaRunner, runner.RunnerVersion},
	} {
		if c.specifier == "" {
			continue
		}
		specifiers, err := pep440.NewSpecifiers(c.specifier)
		if err != nil {
			return fmt.Errorf("capability %q declares an invalid %s specifier %q: %v", name, c.key, c.specifier, err)
		}
		provided, err := pep440.Parse(c.provided)
		if err != nil {
			return fmt.Errorf("runner %s has an unparseable %s version %q: %v",
				runner.RunnerVersion, c.key, c.provided, err)
		}
		if !specifiers.Check(provided) {
			return fmt.Errorf("capability %q requires %s %q but runner %s provides %s %q",
				name, c.key, c.specifier, runner.RunnerVersion, c.key, c.provided)
		}
	}
	return nil
}

var packageNameSeparators = regexp.MustCompile(`[-_.]+`)

// NormalizePackageName applies PEP 503 name normalization: lowercase, with
// runs of "-", "_", "." collapsed to a single "-".
func NormalizePackageName(name string) string {
	return strings.ToLower(packageNameSeparators.ReplaceAllString(name, "-"))
}

// pinPattern matches a bare "name==version" pin (no extras, no markers).
var pinPattern = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)==([A-Za-z0-9._+!-]+)$`)

// ParsePin splits a "name==version" dependency pin, normalizing the name.
func ParsePin(pin string) (name, version string, err error) {
	m := pinPattern.FindStringSubmatch(strings.TrimSpace(pin))
	if m == nil {
		return "", "", fmt.Errorf("invalid dependency pin %q: must be name==version", pin)
	}
	return NormalizePackageName(m[1]), m[2], nil
}

// DetectConflicts unions pinned dependencies across capabilities and the
// runner baseline, returning one message per conflict (deterministic order:
// capabilities and pins are processed in the order given; same-package
// conflicts across capabilities are reported once, after baseline checks).
func DetectConflicts(caps []Deps, runner RunnerInfo) []string {
	var conflicts []string
	type pinned struct {
		capName string
		version string
	}
	seen := map[string]pinned{}
	crossConflicts := map[string]bool{}

	for _, c := range caps {
		for _, pin := range c.Pins {
			pkg, version, err := ParsePin(pin)
			if err != nil {
				conflicts = append(conflicts, fmt.Sprintf("capability %q has an %v", c.Name, err))
				continue
			}

			if baseVersion, ok := runner.Baseline[pkg]; ok && baseVersion != version {
				conflicts = append(conflicts, fmt.Sprintf(
					"capability %q pins %s==%s but the runner %s baseline pins %s==%s",
					c.Name, pkg, version, runner.RunnerVersion, pkg, baseVersion))
			}

			prev, ok := seen[pkg]
			switch {
			case !ok:
				seen[pkg] = pinned{capName: c.Name, version: version}
			case prev.version != version && prev.capName != c.Name && !crossConflicts[pkg]:
				conflicts = append(conflicts, fmt.Sprintf(
					"capabilities %q and %q pin conflicting versions of %s (%s vs %s)",
					prev.capName, c.Name, pkg, prev.version, version))
				crossConflicts[pkg] = true
			}
		}
	}
	return conflicts
}

// ValidateRequires parse-checks a requires tuple without evaluating it
// against a runner (a Capability CR may target a runner release the operator
// doesn't currently embed; compatibility is checked per-Agent attachment).
func ValidateRequires(req Requires) error {
	for _, c := range []struct{ key, specifier string }{
		{"pydantic-ai", req.PydanticAI},
		{"flokoa-runner", req.FlokoaRunner},
	} {
		if c.specifier == "" {
			continue
		}
		if _, err := pep440.NewSpecifiers(c.specifier); err != nil {
			return fmt.Errorf("invalid %s specifier %q: %v", c.key, c.specifier, err)
		}
	}
	return nil
}

// EntryName resolves the compiled-spec capability entry name: the explicit
// serialization-name override when set, else the attr part of the
// "module:attr" entrypoint (pydantic-ai's default serialization name).
func EntryName(entrypoint, override string) string {
	if override != "" {
		return override
	}
	_, attr, _ := strings.Cut(entrypoint, ":")
	return attr
}

// CompileSchema checks that raw bytes are a compilable JSON Schema document.
func CompileSchema(schema []byte) error {
	_, err := compileSchema(schema)
	return err
}

func compileSchema(schema []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return nil, fmt.Errorf("configSchema is not valid JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	const url = "flokoa://capability/config-schema.json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("configSchema is not a JSON Schema document: %v", err)
	}
	compiled, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("configSchema does not compile: %v", err)
	}
	return compiled, nil
}

// ConfigError describes why a capability config failed validation against the
// capability's published schema, with a JSON-pointer path to the offending
// value.
type ConfigError struct {
	Path    string
	Message string
}

func (e *ConfigError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Message + " at " + e.Path
}

// secretPlaceholderRe matches the runtime contract's ${secret:NAME} grammar (§3).
var secretPlaceholderRe = regexp.MustCompile(`\$\{secret:[A-Za-z0-9._-]+\}`)

// ValidateConfig validates a config block against a capability's published
// JSON Schema. Strings containing ${secret:NAME} placeholders satisfy
// string-level constraints (pattern, length, enum, const, format) — admission
// validates shape; values resolve in the runner (product brief §3) — but do
// not satisfy non-string types.
func ValidateConfig(schema, config []byte) error {
	compiled, err := compileSchema(schema)
	if err != nil {
		return err
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(config))
	if err != nil {
		return &ConfigError{Message: fmt.Sprintf("config is not valid JSON: %v", err)}
	}

	placeholders := map[string]bool{}
	collectPlaceholderPaths(doc, "", placeholders)

	if err := compiled.Validate(doc); err != nil {
		verr, ok := err.(*jsonschema.ValidationError)
		if !ok {
			return &ConfigError{Message: err.Error()}
		}
		for _, leaf := range collectLeaves(verr) {
			path := "/" + strings.Join(leaf.InstanceLocation, "/")
			if placeholders[path] && suppressibleForPlaceholder(leaf.ErrorKind) {
				continue
			}
			return &ConfigError{Path: path, Message: leaf.ErrorKind.LocalizedString(errPrinter)}
		}
	}
	return nil
}

// collectPlaceholderPaths records the JSON-pointer path of every string value
// containing a ${secret:NAME} placeholder.
func collectPlaceholderPaths(doc any, path string, into map[string]bool) {
	switch v := doc.(type) {
	case string:
		if secretPlaceholderRe.MatchString(v) {
			if path == "" {
				path = "/"
			}
			into[path] = true
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			collectPlaceholderPaths(v[k], path+"/"+k, into)
		}
	case []any:
		for i, item := range v {
			collectPlaceholderPaths(item, fmt.Sprintf("%s/%d", path, i), into)
		}
	}
}

// suppressibleForPlaceholder reports whether an error kind is a string-level
// constraint a secret placeholder is allowed to bypass. Type errors are never
// suppressed: secrets resolve to strings, so a placeholder satisfies
// `type: string` only.
func suppressibleForPlaceholder(k jsonschema.ErrorKind) bool {
	switch k.(type) {
	case *kind.Pattern, *kind.MinLength, *kind.MaxLength, *kind.Enum, *kind.Const,
		*kind.Format, *kind.ContentEncoding, *kind.ContentMediaType:
		return true
	}
	return false
}

// collectLeaves returns every leaf cause of a validation error tree.
func collectLeaves(v *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(v.Causes) == 0 {
		return []*jsonschema.ValidationError{v}
	}
	var leaves []*jsonschema.ValidationError
	for _, c := range v.Causes {
		leaves = append(leaves, collectLeaves(c)...)
	}
	return leaves
}
