package naming

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/woodleighschool/onomazo/internal/config"
)

// Matcher matches device attributes against patterns.
type Matcher struct {
	patterns map[string][]*regexp.Regexp
}

func newMatcher(cfg config.MatcherConfig) (*Matcher, error) {
	if len(cfg) == 0 {
		return &Matcher{patterns: nil}, nil
	}
	patterns := make(map[string][]*regexp.Regexp, len(cfg))
	for rawKey, values := range cfg {
		key := canonicalMatcherKey(strings.ToLower(strings.TrimSpace(rawKey)))
		if key == "" {
			return nil, fmt.Errorf("matcher key cannot be empty")
		}
		list := values
		if len(list) == 0 {
			return nil, fmt.Errorf("matcher %s has no values", key)
		}
		compiled := make([]*regexp.Regexp, 0, len(list))
		for _, expr := range list {
			re, err := compilePattern(expr)
			if err != nil {
				return nil, fmt.Errorf("matcher %s: %w", key, err)
			}
			compiled = append(compiled, re)
		}
		patterns[key] = compiled
	}
	return &Matcher{patterns: patterns}, nil
}

func (m *Matcher) Matches(attrs map[string]string) bool {
	ok, _ := m.match(attrs, false)
	return ok
}

func (m *Matcher) MatchesWithReason(attrs map[string]string) (bool, string) {
	return m.match(attrs, true)
}

func (m *Matcher) match(attrs map[string]string, wantReason bool) (bool, string) {
	if m == nil || len(m.patterns) == 0 {
		return true, ""
	}
	for key, regexes := range m.patterns {
		value := attrs[key]
		if value == "" {
			if wantReason {
				return false, fmt.Sprintf("attribute %s missing", key)
			}
			return false, ""
		}
		matched := false
		for _, re := range regexes {
			if re.MatchString(value) {
				matched = true
				break
			}
		}
		if !matched {
			if wantReason {
				return false, fmt.Sprintf("attribute %s value %q did not match", key, value)
			}
			return false, ""
		}
	}
	return true, ""
}

func compilePattern(expr string) (*regexp.Regexp, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}
	if strings.HasPrefix(expr, "regex:") {
		return regexp.Compile(expr[len("regex:"):])
	}
	if strings.HasPrefix(expr, "/") && strings.HasSuffix(expr, "/") && len(expr) > 2 {
		return regexp.Compile(expr[1 : len(expr)-1])
	}
	return regexp.Compile("(?i)^" + regexp.QuoteMeta(expr) + "$")
}

func canonicalMatcherKey(key string) string {
	switch key {
	case "platforms":
		return "platform"
	default:
		return key
	}
}
