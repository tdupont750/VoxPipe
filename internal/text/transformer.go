package text

import (
	"errors"
	"fmt"

	"github.com/dlclark/regexp2"
)

// TransformConfig is one ordered text transform rule from config.toml.
type TransformConfig struct {
	Description   string `toml:"description"`
	Find          string `toml:"find"`
	Replace       string `toml:"replace"`
	CaseSensitive bool   `toml:"caseSensitive"`
	IsRegex       bool   `toml:"isRegex"`
}

// Transformer applies transform rules in order.
type Transformer struct {
	rules    []TransformConfig
	replacer *Replacer
}

func NewTransformer(rules []TransformConfig, replacer *Replacer) *Transformer {
	return &Transformer{rules: rules, replacer: replacer}
}

// Transform applies every rule in order. A failing rule (bad pattern, match timeout) is
// skipped so the transcript is not lost; the returned error joins all rule failures.
func (t *Transformer) Transform(text string) (string, error) {
	if text == "" || len(t.rules) == 0 {
		return text, nil
	}

	var errs []error
	result := text

	for _, rule := range t.rules {
		if rule.Find == "" {
			continue
		}

		var out string
		var err error
		if rule.IsRegex {
			options := regexp2.None
			if !rule.CaseSensitive {
				options = regexp2.IgnoreCase
			}
			out, err = t.replacer.Replace(result, rule.Find, rule.Replace, options)
		} else {
			out, err = t.replacer.ReplaceLiteral(result, rule.Find, rule.Replace, !rule.CaseSensitive)
		}

		if err != nil {
			errs = append(errs, fmt.Errorf("transform %q: %w", rule.Description, err))
			continue
		}
		result = out
	}

	return result, errors.Join(errs...)
}
