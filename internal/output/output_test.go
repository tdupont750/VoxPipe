package output

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"voxpipe/internal/config"
)

type recLog struct{ lines []string }

func (l *recLog) Info(m string)  { l.lines = append(l.lines, "I "+m) }
func (l *recLog) Warn(m string)  { l.lines = append(l.lines, "W "+m) }
func (l *recLog) joined() string { return strings.Join(l.lines, "\n") }

type fakeRunner struct {
	calls []Command
	fail  map[string]error // by Command.Name
}

func (f *fakeRunner) run(_ context.Context, c Command) (string, error) {
	f.calls = append(f.calls, c)
	return "boom", f.fail[c.Name]
}

func TestConsolePublish(t *testing.T) {
	var buf bytes.Buffer
	if err := NewConsole(&buf).Publish(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "\nhello\n\n" {
		t.Errorf("got %q", buf.String())
	}
}

func TestYdotoolTypesText(t *testing.T) {
	r, log := &fakeRunner{}, &recLog{}
	if err := NewYdotool(r.run, log).Publish(context.Background(), "hi there"); err != nil {
		t.Fatal(err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("calls: %v", r.calls)
	}
	c := r.calls[0]
	if c.Name != "ydotool" || strings.Join(c.Args, " ") != "type --key-delay=3 --key-hold=2 --file=-" || c.Stdin != "hi there" {
		t.Errorf("bad call: %+v", c)
	}
	if len(c.Env) != 1 || c.Env[0] != "YDOTOOL_SOCKET=/tmp/.ydotool_socket" {
		t.Errorf("env: %v", c.Env)
	}
}

func TestYdotoolBackspaceToken(t *testing.T) {
	r := &fakeRunner{}
	if err := NewYdotool(r.run, &recLog{}).Publish(context.Background(), "{ydotool:KEY_BACKSPACE}word"); err != nil {
		t.Fatal(err)
	}

	if len(r.calls) != 2 {
		t.Fatalf("calls: %v", r.calls)
	}
	if strings.Join(r.calls[0].Args, " ") != "key 14:1 14:0" {
		t.Errorf("first call: %+v", r.calls[0])
	}
	if r.calls[1].Stdin != "word" {
		t.Errorf("remainder: %q", r.calls[1].Stdin)
	}
}

func TestYdotoolBackspaceOnlySkipsType(t *testing.T) {
	r := &fakeRunner{}
	NewYdotool(r.run, &recLog{}).Publish(context.Background(), "{ydotool:KEY_BACKSPACE}")
	if len(r.calls) != 1 {
		t.Errorf("calls: %v", r.calls)
	}
}

func TestYdotoolEmptyTextSkipped(t *testing.T) {
	r, log := &fakeRunner{}, &recLog{}
	if err := NewYdotool(r.run, log).Publish(context.Background(), "  "); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 0 || !strings.Contains(log.joined(), "skipping ydotool") {
		t.Errorf("calls=%v log=%s", r.calls, log.joined())
	}
}

func TestYdotoolExitErrorIncludesCodeAndStderr(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 3").Run()
	r := &fakeRunner{fail: map[string]error{"ydotool": exitErr}}

	err := NewYdotool(r.run, &recLog{}).Publish(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "code 3") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("got %v", err)
	}
}

func clip(r *fakeRunner, installed ...string) *Clipboard {
	c := NewClipboard(r.run, &recLog{})
	c.lookPath = func(name string) (string, error) {
		for _, i := range installed {
			if i == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
	return c
}

func TestClipboardUsesFirstInstalledTool(t *testing.T) {
	r := &fakeRunner{}
	if err := clip(r, "xclip", "xsel").Publish(context.Background(), "text"); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || r.calls[0].Name != "xclip" || r.calls[0].Stdin != "text" {
		t.Errorf("calls: %+v", r.calls)
	}
}

func TestClipboardFallsThroughOnFailure(t *testing.T) {
	r := &fakeRunner{fail: map[string]error{"wl-copy": errors.New("no wayland")}}
	if err := clip(r, "wl-copy", "xclip").Publish(context.Background(), "text"); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 || r.calls[1].Name != "xclip" {
		t.Errorf("calls: %+v", r.calls)
	}
}

func TestClipboardErrors(t *testing.T) {
	if err := clip(&fakeRunner{}).Publish(context.Background(), "text"); err == nil {
		t.Error("expected error when no tool installed")
	}

	r := &fakeRunner{fail: map[string]error{"xsel": errors.New("bad")}}
	if err := clip(r, "xsel").Publish(context.Background(), "text"); err == nil {
		t.Error("expected error when the only tool fails")
	}

	r = &fakeRunner{}
	if err := clip(r, "xsel").Publish(context.Background(), " "); err != nil || len(r.calls) != 0 {
		t.Errorf("empty text must be skipped: %v %v", err, r.calls)
	}
}

type stubPublisher struct {
	name string
	err  error
	got  []string
}

func (s *stubPublisher) Name() string { return s.name }
func (s *stubPublisher) Publish(_ context.Context, text string) error {
	s.got = append(s.got, text)
	return s.err
}

func TestDispatcherContinuesPastFailure(t *testing.T) {
	bad := &stubPublisher{name: "bad", err: errors.New("nope")}
	good := &stubPublisher{name: "good"}
	log := &recLog{}

	d, err := NewDispatcher([]Publisher{bad, good}, log)
	if err != nil {
		t.Fatal(err)
	}
	d.Dispatch(context.Background(), "text")

	if len(good.got) != 1 || good.got[0] != "text" {
		t.Errorf("good publisher not reached: %v", good.got)
	}
	if !strings.Contains(log.joined(), "Output bad failed: nope") {
		t.Errorf("failure not logged: %s", log.joined())
	}
}

func TestDispatcherRequiresPublisher(t *testing.T) {
	if _, err := NewDispatcher(nil, &recLog{}); err == nil {
		t.Error("expected error")
	}
}

func TestBuildDedupesAndMaps(t *testing.T) {
	pubs, err := Build([]config.OutputTarget{config.OutputConsole, config.OutputYdotool, config.OutputConsole, config.OutputClipboard}, &bytes.Buffer{}, &recLog{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range pubs {
		names = append(names, p.Name())
	}
	if strings.Join(names, ",") != "console,ydotool,clipboard" {
		t.Errorf("got %v", names)
	}

	if _, err := Build([]config.OutputTarget{"Printer"}, &bytes.Buffer{}, &recLog{}); err == nil {
		t.Error("unknown target must fail")
	}
}
