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
// capabilities and pins are processed in the order given; baseline collisions
// and intra-capability contradictions are reported as they are seen,
// cross-capability conflicts against the first capability that pinned the
// package). Conflicts are reported even when two Deps entries share a Name —
// callers key Deps by namespaced identity so the names disambiguate.
func DetectConflicts(caps []Deps, runner RunnerInfo) []string {
	var conflicts []string
	type pinned struct {
		idx     int
		capName string
		version string
	}
	established := map[string]pinned{} // first capability to pin each package
	reportedCross := map[string]bool{} // package + current index, so each pair reports once

	for idx, c := range caps {
		local := map[string]string{} // this capability's own pins, for intra checks
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

			// Intra-capability contradiction: the same package pinned twice at
			// different versions within one capability.
			if prevVersion, ok := local[pkg]; ok {
				if prevVersion != version {
					conflicts = append(conflicts, fmt.Sprintf(
						"capability %q pins %s at conflicting versions (%s and %s)",
						c.Name, pkg, prevVersion, version))
				}
			} else {
				local[pkg] = version
			}

			// Cross-capability conflict against the first capability that
			// pinned this package. Distinct Deps entries conflict even when
			// they share a Name (namespaced aliasing must not hide it).
			prev, ok := established[pkg]
			switch {
			case !ok:
				established[pkg] = pinned{idx: idx, capName: c.Name, version: version}
			case prev.idx != idx && prev.version != version:
				crossKey := fmt.Sprintf("%s\x00%d", pkg, idx)
				if !reportedCross[crossKey] {
					conflicts = append(conflicts, fmt.Sprintf(
						"capabilities %q and %q pin conflicting versions of %s (%s vs %s)",
						prev.capName, c.Name, pkg, prev.version, version))
					reportedCross[crossKey] = true
				}
			}
		}
	}
	return conflicts
}

// PlatformCapabilityPrefix is reserved for operator-injected capability
// entries; user-published Capabilities may not claim a name under it.
const PlatformCapabilityPrefix = "flokoa.platform/"

// ValidateEntryName checks a compiled-spec capability entry name (the
// serialization name a Capability contributes). It mirrors the inline-fragment
// rule: no module/class path punctuation, and the operator-injected
// flokoa.platform/ prefix is reserved.
func ValidateEntryName(name string) error {
	if name == "" {
		return fmt.Errorf("capability entry name must not be empty")
	}
	if strings.HasPrefix(name, PlatformCapabilityPrefix) {
		return fmt.Errorf("capability entry name %q uses the reserved %q prefix (operator-injected capabilities only)", name, PlatformCapabilityPrefix)
	}
	if strings.ContainsAny(name, "./:") {
		return fmt.Errorf("capability entry name %q must not contain '.', '/', or ':' (it is the pydantic-ai capability class name)", name)
	}
	return nil
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

// denySchemaLoader refuses every external schema reference. A configSchema
// comes verbatim from an untrusted CR; santhosh-tekuri's default loader reads
// file:// $refs from the operator pod's filesystem, so we deny all external
// resolution — only the in-memory resource and intra-document #/… pointers
// resolve.
type denySchemaLoader struct{}

func (denySchemaLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema reference %q is not allowed", url)
}

func compileSchema(schema []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return nil, fmt.Errorf("configSchema is not valid JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(denySchemaLoader{})
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
		if placeholderSatisfied(verr, placeholders) {
			return nil
		}
		leaf := firstGenuineLeaf(verr, placeholders)
		return &ConfigError{
			Path:    "/" + joinPointer(leaf.InstanceLocation),
			Message: leaf.ErrorKind.LocalizedString(errPrinter),
		}
	}
	return nil
}

// placeholderSatisfied reports whether a validation error tree is fully
// excused by ${secret:NAME} placeholders. The walk is composition-aware: an
// anyOf/oneOf node passes if ANY branch is satisfied (so "integer or
// secret-string" accepts a placeholder via the string branch); every other
// node passes only if ALL its causes are satisfied; a leaf passes only if the
// instance is a placeholder and the failed constraint is a suppressible
// string-level one.
func placeholderSatisfied(v *jsonschema.ValidationError, placeholders map[string]bool) bool {
	switch v.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf:
		for _, c := range v.Causes {
			if placeholderSatisfied(c, placeholders) {
				return true
			}
		}
		return false
	}
	if len(v.Causes) == 0 {
		return placeholders["/"+joinPointer(v.InstanceLocation)] && suppressibleForPlaceholder(v.ErrorKind)
	}
	for _, c := range v.Causes {
		if !placeholderSatisfied(c, placeholders) {
			return false
		}
	}
	return true
}

// firstGenuineLeaf returns a leaf that is not excused by a placeholder, to
// report. Inside an unsatisfied anyOf/oneOf it descends the branch with the
// deepest instance location (the alternative the config actually attempted).
func firstGenuineLeaf(v *jsonschema.ValidationError, placeholders map[string]bool) *jsonschema.ValidationError {
	switch v.ErrorKind.(type) {
	case *kind.AnyOf, *kind.OneOf:
		deepest, depth := v, -1
		for _, c := range v.Causes {
			leaf := firstGenuineLeaf(c, placeholders)
			if d := len(leaf.InstanceLocation); d > depth {
				deepest, depth = leaf, d
			}
		}
		return deepest
	}
	if len(v.Causes) == 0 {
		return v
	}
	for _, c := range v.Causes {
		if !placeholderSatisfied(c, placeholders) {
			return firstGenuineLeaf(c, placeholders)
		}
	}
	return v
}

// escapePointerToken applies RFC 6901 escaping so a property key containing
// '/' or '~' cannot collide with a structural path segment.
func escapePointerToken(t string) string {
	return strings.ReplaceAll(strings.ReplaceAll(t, "~", "~0"), "/", "~1")
}

// joinPointer builds an RFC 6901 pointer body (no leading slash) from a token
// slice. Both placeholder collection and error reporting use it, so a key with
// a '/' maps to the same escaped path on both sides.
func joinPointer(tokens []string) string {
	escaped := make([]string, len(tokens))
	for i, t := range tokens {
		escaped[i] = escapePointerToken(t)
	}
	return strings.Join(escaped, "/")
}

// collectPlaceholderPaths records the JSON-pointer path of every string value
// containing a ${secret:NAME} placeholder (paths RFC 6901-escaped to match
// the validator's instance locations).
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
			collectPlaceholderPaths(v[k], path+"/"+escapePointerToken(k), into)
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
// `type: string` only. Enum/Const are suppressed only when a string value is
// actually allowed — a placeholder cannot satisfy an all-numeric enum.
func suppressibleForPlaceholder(k jsonschema.ErrorKind) bool {
	switch e := k.(type) {
	case *kind.Pattern, *kind.MinLength, *kind.MaxLength, *kind.Format,
		*kind.ContentEncoding, *kind.ContentMediaType:
		return true
	case *kind.Enum:
		for _, want := range e.Want {
			if _, ok := want.(string); ok {
				return true
			}
		}
		return false
	case *kind.Const:
		_, ok := e.Want.(string)
		return ok
	}
	return false
}
