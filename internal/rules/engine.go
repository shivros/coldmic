package rules

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type compiledRule interface {
	Apply(input string) (output string, changed bool)
}

// RuleParser parses one line into a compiled rule.
type RuleParser interface {
	CanParse(line string) bool
	Parse(line string) (compiledRule, error)
}

// Engine applies deterministic substitutions loaded from a rules file.
type Engine struct {
	rules     []compiledRule
	loopLimit int
}

// NewEngine loads and compiles rules from a file using built-in parsers.
func NewEngine(path string, loopLimit int) (*Engine, error) {
	return NewEngineWithParsers(path, loopLimit, defaultRuleParsers())
}

// NewEngineWithParsers allows parser extension without engine changes.
func NewEngineWithParsers(path string, loopLimit int, parsers []RuleParser) (*Engine, error) {
	if loopLimit <= 0 {
		loopLimit = 30
	}
	if len(parsers) == 0 {
		parsers = defaultRuleParsers()
	}

	if strings.TrimSpace(path) == "" {
		return &Engine{loopLimit: loopLimit}, nil
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Engine{loopLimit: loopLimit}, nil
		}
		return nil, fmt.Errorf("failed to read rules file %q: %w", path, err)
	}

	rules, err := parseRules(string(contents), parsers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rules file %q: %w", path, err)
	}

	return &Engine{rules: rules, loopLimit: loopLimit}, nil
}

// Apply transforms text deterministically.
func (e *Engine) Apply(text string) (string, error) {
	if len(e.rules) == 0 {
		return text, nil
	}

	result := text
	for i := 0; i < e.loopLimit; i++ {
		changed := false
		for _, rule := range e.rules {
			next, ruleChanged := rule.Apply(result)
			if ruleChanged {
				result = next
				changed = true
			}
		}
		if !changed {
			return result, nil
		}
	}

	return result, nil
}

func parseRules(contents string, parsers []RuleParser) ([]compiledRule, error) {
	lines := strings.Split(contents, "\n")
	rules := make([]compiledRule, 0, len(lines))

	for index, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parsed := false
		for _, parser := range parsers {
			if !parser.CanParse(line) {
				continue
			}
			rule, err := parser.Parse(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", index+1, err)
			}
			rules = append(rules, rule)
			parsed = true
			break
		}

		if !parsed {
			return nil, fmt.Errorf("line %d: unsupported rule format", index+1)
		}
	}

	return rules, nil
}

func defaultRuleParsers() []RuleParser {
	return []RuleParser{regexRuleParser{}, literalRuleParser{}}
}

type literalRuleParser struct{}

func (literalRuleParser) CanParse(line string) bool {
	return strings.Contains(line, "=>")
}

func (literalRuleParser) Parse(line string) (compiledRule, error) {
	return parseLiteralRule(line)
}

type regexRuleParser struct{}

func (regexRuleParser) CanParse(line string) bool {
	return looksLikeRegexRule(line)
}

func (regexRuleParser) Parse(line string) (compiledRule, error) {
	return parseRegexRule(line)
}

type literalRule struct {
	replacement string
	re          *regexp.Regexp
}

func parseLiteralRule(line string) (compiledRule, error) {
	parts := strings.SplitN(line, "=>", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid literal rule")
	}
	from := strings.TrimSpace(parts[0])
	to := strings.TrimSpace(parts[1])
	if from == "" {
		return nil, errors.New("literal rule source cannot be empty")
	}

	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(from))
	if err != nil {
		return nil, fmt.Errorf("invalid literal source: %w", err)
	}

	return literalRule{replacement: to, re: re}, nil
}

func (r literalRule) Apply(input string) (string, bool) {
	output := r.re.ReplaceAllString(input, r.replacement)
	return output, output != input
}

type regexRule struct {
	re          *regexp.Regexp
	replacement string
	global      bool
}

func parseRegexRule(line string) (compiledRule, error) {
	if len(line) < 2 {
		return nil, errors.New("invalid regex rule")
	}
	delim := line[1]
	if isAlphaNumericOrSpace(delim) {
		return nil, errors.New("regex delimiter must be non-alphanumeric")
	}

	pattern, pos, err := parseDelimited(line, 2, delim)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}
	replacement, pos, err := parseDelimited(line, pos, delim)
	if err != nil {
		return nil, fmt.Errorf("invalid regex replacement: %w", err)
	}
	flags := strings.TrimSpace(line[pos:])

	flagState := struct {
		ignoreCase bool
		global     bool
		multiLine  bool
		dotAll     bool
	}{
		ignoreCase: true,
		global:     false,
	}

	for _, flag := range flags {
		switch flag {
		case 'i':
			flagState.ignoreCase = true
		case 'g':
			flagState.global = true
		case 'm':
			flagState.multiLine = true
		case 's':
			flagState.dotAll = true
		case ' ':
			continue
		default:
			return nil, fmt.Errorf("unsupported regex flag %q", flag)
		}
	}

	prefixFlags := ""
	if flagState.ignoreCase {
		prefixFlags += "i"
	}
	if flagState.multiLine {
		prefixFlags += "m"
	}
	if flagState.dotAll {
		prefixFlags += "s"
	}
	if prefixFlags != "" {
		pattern = "(?" + prefixFlags + ")" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %w", err)
	}

	return regexRule{re: re, replacement: replacement, global: flagState.global}, nil
}

func (r regexRule) Apply(input string) (string, bool) {
	if r.global {
		output := r.re.ReplaceAllString(input, r.replacement)
		return output, output != input
	}

	loc := r.re.FindStringIndex(input)
	if loc == nil {
		return input, false
	}

	segment := input[loc[0]:loc[1]]
	replaced := r.re.ReplaceAllString(segment, r.replacement)
	output := input[:loc[0]] + replaced + input[loc[1]:]
	return output, output != input
}

func parseDelimited(line string, start int, delim byte) (string, int, error) {
	if start >= len(line) {
		return "", 0, errors.New("unexpected end of expression")
	}

	var builder strings.Builder
	escaped := false
	for index := start; index < len(line); index++ {
		char := line[index]
		if escaped {
			builder.WriteByte(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			builder.WriteByte(char)
			continue
		}
		if char == delim {
			return builder.String(), index + 1, nil
		}
		builder.WriteByte(char)
	}
	return "", 0, errors.New("unterminated expression")
}

func isAlphaNumericOrSpace(char byte) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == ' ' || char == '\t'
}

func looksLikeRegexRule(line string) bool {
	return len(line) > 1 && line[0] == 's' && !isAlphaNumericOrSpace(line[1])
}
