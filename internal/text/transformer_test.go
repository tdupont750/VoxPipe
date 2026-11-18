package text

import "testing"

func TestTransformerAppliesRulesInOrder(t *testing.T) {
	tr := NewTransformer([]TransformConfig{
		{Find: "a", Replace: "b"},
		{Find: "b", Replace: "c"},
	}, NewReplacer())

	got, err := tr.Transform("a")
	if err != nil {
		t.Fatal(err)
	}
	if got != "c" {
		t.Errorf("got %q, want c", got)
	}
}

func TestTransformerCaseSensitivity(t *testing.T) {
	tests := []struct {
		name string
		rule TransformConfig
		in   string
		want string
	}{
		{"literal insensitive", TransformConfig{Find: "sink", Replace: "sync"}, "SINK it", "sync it"},
		{"literal sensitive", TransformConfig{Find: "sink", Replace: "sync", CaseSensitive: true}, "SINK sink", "SINK sync"},
		{"regex insensitive", TransformConfig{Find: `\bgira\b`, Replace: "Jira", IsRegex: true}, "open GIRA now", "open Jira now"},
		{"regex sensitive", TransformConfig{Find: `\bgira\b`, Replace: "Jira", IsRegex: true, CaseSensitive: true}, "GIRA gira", "GIRA Jira"},
		{"regex template tokens", TransformConfig{Find: `^space\s(\w)`, Replace: "{1:ToUpper}", IsRegex: true}, "Space hello", "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTransformer([]TransformConfig{tt.rule}, NewReplacer()).Transform(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTransformerEmptyInputsAndRules(t *testing.T) {
	tr := NewTransformer([]TransformConfig{{Find: "", Replace: "x"}}, NewReplacer())

	if got, _ := tr.Transform(""); got != "" {
		t.Errorf("empty text: got %q", got)
	}
	if got, _ := tr.Transform("keep"); got != "keep" {
		t.Errorf("empty find must be skipped: got %q", got)
	}
	if got, _ := NewTransformer(nil, NewReplacer()).Transform("keep"); got != "keep" {
		t.Errorf("no rules: got %q", got)
	}
}

func TestTransformerBadRuleIsSkippedAndReported(t *testing.T) {
	tr := NewTransformer([]TransformConfig{
		{Description: "bad", Find: "(unclosed", Replace: "x", IsRegex: true},
		{Description: "good", Find: "a", Replace: "b"},
	}, NewReplacer())

	got, err := tr.Transform("a")
	if got != "b" {
		t.Errorf("good rule must still apply: got %q", got)
	}
	if err == nil {
		t.Error("expected error for bad rule")
	}
}
