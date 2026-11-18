package audio

import (
	"strings"
	"testing"
)

type recLog struct{ lines []string }

func (l *recLog) Debug(m string) { l.lines = append(l.lines, "D "+m) }
func (l *recLog) Info(m string)  { l.lines = append(l.lines, "I "+m) }
func (l *recLog) Warn(m string)  { l.lines = append(l.lines, "W "+m) }
func (l *recLog) Error(m string) { l.lines = append(l.lines, "E "+m) }

func (l *recLog) has(prefix, sub string) bool {
	for _, s := range l.lines {
		if strings.HasPrefix(s, prefix) && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestResolveByNameCaseInsensitive(t *testing.T) {
	log := &recLog{}
	r := NewResolver("c922 pro", -1, log)

	got := r.ResolveFrom([]string{"Built-in", "HD Pro Webcam C922 Pro Stream"})
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}
	if !log.has("I ", "found at index 1") {
		t.Errorf("missing info log: %v", log.lines)
	}
}

func TestResolveFallsBackWhenNameMissing(t *testing.T) {
	log := &recLog{}
	r := NewResolver("nope", 2, log)

	if got := r.ResolveFrom([]string{"a", "b", "c"}); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if !log.has("W ", "NOT found, falling back to index 2") {
		t.Errorf("missing warn: %v", log.lines)
	}
}

func TestResolveNoNameUsesFallbackSilently(t *testing.T) {
	log := &recLog{}
	r := NewResolver("  ", -1, log)

	if got := r.ResolveFrom([]string{"a"}); got != -1 {
		t.Errorf("got %d, want -1", got)
	}
	if len(log.lines) != 0 {
		t.Errorf("unexpected logs: %v", log.lines)
	}
}

func TestResolveLogsIndexChange(t *testing.T) {
	log := &recLog{}
	r := NewResolver("cam", -1, log)

	r.ResolveFrom([]string{"cam"})
	r.ResolveFrom([]string{"cam"}) // unchanged: no extra log
	if len(log.lines) != 1 {
		t.Fatalf("unchanged index must not log again: %v", log.lines)
	}

	r.ResolveFrom([]string{"other", "cam"})
	if !log.has("W ", "index changed from 0 to 1") {
		t.Errorf("missing change warn: %v", log.lines)
	}

	r.ResolveFrom([]string{"other"})
	if !log.has("W ", "no longer found; index changed from 1 to fallback -1") {
		t.Errorf("missing lost warn: %v", log.lines)
	}
}

func TestLogAvailable(t *testing.T) {
	log := &recLog{}
	r := NewResolver("b", -1, log)
	r.LogAvailable([]string{"a", "b"})

	if !log.has("D ", "[1] b") || !log.has("D ", "Using device index 1 -> b") {
		t.Errorf("got %v", log.lines)
	}
}
