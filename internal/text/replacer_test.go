package text

import (
	"testing"

	"github.com/dlclark/regexp2"
)

func TestReplace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pattern  string
		template string
		want     string
	}{
		{"uppercases first char after space", "Space hello world", `^Space\s(\w)`, "{1:ToUpper}", "Hello world"},
		{"lowercases first char after tag", "[TAG] Hello World", `^\[TAG\]\s+(\w)`, "{1:ToLower}", "hello World"},
		{"multiple groups and transforms", "Name: JOHN DOE", `Name:\s+(\w+)\s+(\w+)`, "{1:ToLower} {2:ToUpper}", "john DOE"},
		{"no transform inserts group", "abc123", `([a-z]+)(\d+)`, "{2}-{1}", "123-abc"},
		{"no match returns input", "no match here", `^Space\s(\w)`, "{1:ToUpper}", "no match here"},
		{"out of range group is empty", "abc123", `([a-z]+)(\d+)`, "{3}-{1}", "-abc"},
		{"group zero is whole match", "abc123", `[a-z]+`, "<{0}>", "<abc>123"},
	}

	r := NewReplacer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Replace(tt.input, tt.pattern, tt.template, regexp2.None)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplaceRepeatedCallsDoNotReuseStaleState(t *testing.T) {
	r := NewReplacer()
	pattern, tmpl := `^Space\s(\w)`, "{1:ToUpper}"

	cases := []struct{ in, want string }{
		{"Space alpha", "Alpha"},
		{"Space bravo", "Bravo"},
		{"no match here", "no match here"},
		{"Space alpha", "Alpha"},
	}
	for _, c := range cases {
		got, err := r.Replace(c.in, pattern, tmpl, regexp2.None)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("Replace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReplaceSamePatternDifferentOptionsCachedSeparately(t *testing.T) {
	r := NewReplacer()
	pattern, tmpl := `^space\s(\w)`, "{1:ToUpper}"

	sensitive, _ := r.Replace("Space alpha", pattern, tmpl, regexp2.None)
	insensitive, _ := r.Replace("Space alpha", pattern, tmpl, regexp2.IgnoreCase)

	if sensitive != "Space alpha" {
		t.Errorf("case sensitive: got %q", sensitive)
	}
	if insensitive != "Alpha" {
		t.Errorf("ignore case: got %q", insensitive)
	}
}

func TestReplaceInvalidPatternReturnsError(t *testing.T) {
	r := NewReplacer()
	got, err := r.Replace("input", `(unclosed`, "x", regexp2.None)
	if err == nil {
		t.Fatal("expected error")
	}
	if got != "input" {
		t.Errorf("got %q, want input unchanged", got)
	}
}

func TestReplaceCatastrophicPatternTimesOut(t *testing.T) {
	r := NewReplacer()
	input := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa!"
	_, err := r.Replace(input, `^(a+)+$`, "x", regexp2.None)
	if err == nil {
		t.Fatal("expected match timeout error")
	}
}

func TestReplaceCacheCapStillWorks(t *testing.T) {
	r := NewReplacer()
	for i := 0; i < maxCachedPatterns+10; i++ {
		pattern := regexp2.Escape(string(rune('a'+i%26))) + string(rune('0'+i%10)) + `?x` + string(rune('A'+i/26))
		if _, err := r.Replace("zzz", pattern, "", regexp2.None); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.cache) > maxCachedPatterns {
		t.Errorf("cache size %d exceeds cap %d", len(r.cache), maxCachedPatterns)
	}
}

func TestReplaceLiteral(t *testing.T) {
	r := NewReplacer()

	got, err := r.ReplaceLiteral("Foo.bar foo.BAR", "foo.bar", "{1}", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{1} {1}" {
		t.Errorf("ignore case: got %q", got)
	}

	got, _ = r.ReplaceLiteral("Foo.bar foo.bar", "foo.bar", "x", false)
	if got != "Foo.bar x" {
		t.Errorf("case sensitive: got %q", got)
	}

	// '.' must be literal, not a wildcard
	got, _ = r.ReplaceLiteral("fooxbar", "foo.bar", "x", true)
	if got != "fooxbar" {
		t.Errorf("metacharacters must be escaped: got %q", got)
	}
}
