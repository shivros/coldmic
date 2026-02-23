package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngineLiteralAndRegexRules(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	rulesPath := filepath.Join(tmpDir, "substitutions.rules")

	rules := `
# literal
pull request => PR
# regex with default case-insensitive
s/\bdeep\s*gram\b/Deepgram/g
`

	if err := os.WriteFile(rulesPath, []byte(rules), 0o600); err != nil {
		t.Fatalf("failed to write rules file: %v", err)
	}

	engine, err := NewEngine(rulesPath, 30)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	output, err := engine.Apply("deep gram pull request")
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if output != "Deepgram PR" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestEngineIteratesUntilStable(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	rulesPath := filepath.Join(tmpDir, "substitutions.rules")

	rules := `
a => b
b => c
`

	if err := os.WriteFile(rulesPath, []byte(rules), 0o600); err != nil {
		t.Fatalf("failed to write rules file: %v", err)
	}

	engine, err := NewEngine(rulesPath, 5)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	output, err := engine.Apply("a")
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if output != "c" {
		t.Fatalf("expected c, got %q", output)
	}
}

func TestEngineLiteralRuleStartingWithS(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	rulesPath := filepath.Join(tmpDir, "substitutions.rules")

	rules := `
solid complaint => SOLID-compliant
`

	if err := os.WriteFile(rulesPath, []byte(rules), 0o600); err != nil {
		t.Fatalf("failed to write rules file: %v", err)
	}

	engine, err := NewEngine(rulesPath, 30)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	output, err := engine.Apply("solid complaint plan")
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if output != "SOLID-compliant plan" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestEngineSupportsParserExtension(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	rulesPath := filepath.Join(tmpDir, "substitutions.rules")

	rules := `
prefix:Hello=>Howdy
`

	if err := os.WriteFile(rulesPath, []byte(rules), 0o600); err != nil {
		t.Fatalf("failed to write rules file: %v", err)
	}

	parsers := append([]RuleParser{prefixRuleParser{}}, defaultRuleParsers()...)
	engine, err := NewEngineWithParsers(rulesPath, 5, parsers)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	output, err := engine.Apply("hello world")
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if output != "Howdy world" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestRegexRuleWithoutGlobalReplacesFirstMatchOnly(t *testing.T) {
	t.Parallel()

	rule, err := parseRegexRule(`s/foo/bar/`)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	output, changed := rule.Apply("foo foo")
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if output != "bar foo" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestParseRegexRuleUnsupportedFlag(t *testing.T) {
	t.Parallel()

	_, err := parseRegexRule(`s/foo/bar/x`)
	if err == nil {
		t.Fatalf("expected unsupported flag error")
	}
}

func TestParseRulesUnsupportedLine(t *testing.T) {
	t.Parallel()

	_, err := parseRules("not-a-rule", defaultRuleParsers())
	if err == nil {
		t.Fatalf("expected unsupported rule format error")
	}
}

type prefixRuleParser struct{}

func (prefixRuleParser) CanParse(line string) bool {
	return strings.HasPrefix(line, "prefix:")
}

func (prefixRuleParser) Parse(line string) (compiledRule, error) {
	payload := strings.TrimPrefix(line, "prefix:")
	parts := strings.SplitN(payload, "=>", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid prefix rule")
	}
	return parseLiteralRule(parts[0] + " => " + parts[1])
}
