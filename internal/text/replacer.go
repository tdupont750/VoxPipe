// Package text applies the configured transcript transforms.
package text

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
)

// maxCachedPatterns bounds the pattern cache. Patterns come from AppConfig.json and are
// fixed for the process lifetime, so the cap only guards against unbounded callers.
const maxCachedPatterns = 64

// matchTimeout guards against catastrophic backtracking in user supplied patterns.
const matchTimeout = time.Second

// tokenRegex matches replacement tokens: {1}, {2:ToUpper}, {3:ToLower}.
var tokenRegex = regexp.MustCompile(`\{(\d+)(?::(ToUpper|ToLower))?\}`)

type cacheKey struct {
	pattern string
	options regexp2.RegexOptions
}

// Replacer performs regex replacement with group-token templates and caches compiled patterns.
type Replacer struct {
	mu    sync.Mutex
	cache map[cacheKey]*regexp2.Regexp
}

func NewReplacer() *Replacer {
	return &Replacer{cache: make(map[cacheKey]*regexp2.Regexp)}
}

// Replace replaces every match of pattern in input with template, expanding tokens
// {N}, {N:ToUpper}, {N:ToLower}. An out-of-range N expands to the empty string.
func (r *Replacer) Replace(input, pattern, template string, options regexp2.RegexOptions) (string, error) {
	re, err := r.get(pattern, options)
	if err != nil {
		return input, err
	}

	out, err := re.ReplaceFunc(input, func(m regexp2.Match) string {
		return expandTemplate(template, &m)
	}, -1, -1)
	if err != nil {
		return input, fmt.Errorf("pattern %q: %w", pattern, err)
	}
	return out, nil
}

// ReplaceLiteral replaces every occurrence of find with replacement, with no token expansion.
func (r *Replacer) ReplaceLiteral(input, find, replacement string, ignoreCase bool) (string, error) {
	options := regexp2.None
	if ignoreCase {
		options = regexp2.IgnoreCase
	}
	re, err := r.get(regexp2.Escape(find), options)
	if err != nil {
		return input, err
	}

	out, err := re.ReplaceFunc(input, func(regexp2.Match) string { return replacement }, -1, -1)
	if err != nil {
		return input, fmt.Errorf("literal %q: %w", find, err)
	}
	return out, nil
}

func (r *Replacer) get(pattern string, options regexp2.RegexOptions) (*regexp2.Regexp, error) {
	key := cacheKey{pattern, options}

	r.mu.Lock()
	cached, ok := r.cache[key]
	full := len(r.cache) >= maxCachedPatterns
	r.mu.Unlock()
	if ok {
		return cached, nil
	}

	re, err := regexp2.Compile(pattern, options)
	if err != nil {
		return nil, fmt.Errorf("pattern %q: %w", pattern, err)
	}
	re.MatchTimeout = matchTimeout

	// Past the cap, return an uncached instance rather than growing without bound.
	if !full {
		r.mu.Lock()
		r.cache[key] = re
		r.mu.Unlock()
	}
	return re, nil
}

func expandTemplate(template string, m *regexp2.Match) string {
	groups := m.Groups()
	return tokenRegex.ReplaceAllStringFunc(template, func(token string) string {
		sub := tokenRegex.FindStringSubmatch(token)
		idx, err := strconv.Atoi(sub[1])
		if err != nil || idx >= len(groups) {
			return ""
		}

		g := m.GroupByNumber(idx)
		if g == nil {
			return ""
		}
		value := g.String()

		switch sub[2] {
		case "ToUpper":
			return strings.ToUpper(value)
		case "ToLower":
			return strings.ToLower(value)
		default:
			return value
		}
	})
}
