package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// SecretPlaceholder formats a ${secret:NAME} placeholder (runtime contract §3).
func SecretPlaceholder(name string) string {
	return fmt.Sprintf("${secret:%s}", name)
}

// PlaceholderPattern matches ${secret:NAME} placeholders in compiled specs.
var PlaceholderPattern = regexp.MustCompile(`\$\{secret:([A-Za-z0-9._-]+)\}`)

var nonAlnum = regexp.MustCompile(`[^A-Z0-9]`)

// SecretEnvName maps a secret reference name to its environment variable per
// the runtime contract's normalization rule (§3): uppercase, every character
// outside [A-Z0-9] becomes "_", prefixed FLOKOA_SECRET_. The Python runner
// implements the same rule; golden pairs in testdata/secret-env-pairs.json
// are asserted on both sides.
func SecretEnvName(name string) string {
	normalized := nonAlnum.ReplaceAllString(strings.ToUpper(name), "_")
	return "FLOKOA_SECRET_" + normalized
}
