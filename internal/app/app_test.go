package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCapture struct {
	mu        sync.Mutex
	capturing bool
	samples   []int16
	startErr  error
	starts    int
	discards  int
}

func (f *fakeCapture) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	f.capturing = true
	f.starts++
	return nil
}

func (f *fakeCapture) StopAndGet() ([]int16, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.capturing {
		return nil, nil
	}
	f.capturing = false
	return f.samples, nil
}

func (f *fakeCapture) StopAndDiscard() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.capturing {
		f.discards++
	}
	f.capturing = false
}

func (f *fakeCapture) IsCapturing() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.capturing
}

type fakeTranscriber struct {
	text  string
	err   error
	calls int
}

func (f *fakeTranscriber) Transcribe(context.Context, []int16) (string, error) {
	f.calls++
	return f.text, f.err
}

type fakeDispatcher struct {
	mu  sync.Mutex
	got []string
}

func (f *fakeDispatcher) Dispatch(_ context.Context, text string) {
	f.mu.Lock()
	f.got = append(f.got, text)
	f.mu.Unlock()
}

func (f *fakeDispatcher) texts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

type recLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *recLog) add(s string)   { l.mu.Lock(); l.lines = append(l.lines, s); l.mu.Unlock() }
func (l *recLog) Debug(m string) { l.add("D " + m) }
func (l *recLog) Info(m string)  { l.add("I " + m) }
func (l *recLog) Warn(m string)  { l.add("W " + m) }
func (l *recLog) Error(m string) { l.add("E " + m) }
func (l *recLog) ToggleAlert(_ string, show bool) {
	if show {
		l.add("ALERT on")
	} else {
		l.add("ALERT off")
	}
}
func (l *recLog) has(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.lines {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

type harness struct {
	cap  *fakeCapture
	tr   *fakeTranscriber
	disp *fakeDispatcher
	log  *recLog
	cmds chan rune
	done chan struct{}
	ctx  context.Context
}

func start(t *testing.T, timeout time.Duration) *harness {
	t.Helper()
	h := &harness{
		cap:  &fakeCapture{samples: []int16{1, 2, 3}},
		tr:   &fakeTranscriber{text: "hello world"},
		disp: &fakeDispatcher{},
		log:  &recLog{},
		cmds: make(chan rune, 16),
		done: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.ctx = ctx
	a := New(h.cap, h.tr, h.disp, timeout, h.log)
	go func() { a.Run(ctx, cancel, h.cmds); close(h.done) }()
	t.Cleanup(func() { cancel(); <-h.done })
	return h
}

func (h *harness) send(cs ...rune) {
	for _, c := range cs {
		h.cmds <- c
	}
}

// quit sends 'q' and waits for the loop to finish, so all earlier commands have been handled.
func (h *harness) quit(t *testing.T) {
	t.Helper()
	h.send('q')
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not exit on q")
	}
}

func TestToggleStartsThenTranscribes(t *testing.T) {
	h := start(t, time.Minute)
	h.send('v', 'v')
	h.quit(t)

	if got := h.disp.texts(); len(got) != 1 || got[0] != "hello world" {
		t.Errorf("dispatched %v", got)
	}
	if h.cap.starts != 1 {
		t.Errorf("starts = %d", h.cap.starts)
	}
}

func TestUpperCaseCommandsAccepted(t *testing.T) {
	h := start(t, time.Minute)
	h.send('C', 'X')
	h.quit(t)
	if len(h.disp.texts()) != 1 {
		t.Errorf("dispatched %v", h.disp.texts())
	}
}

func TestDiscardDoesNotTranscribe(t *testing.T) {
	h := start(t, time.Minute)
	h.send('c', 'z')
	h.quit(t)

	if h.tr.calls != 0 || len(h.disp.texts()) != 0 {
		t.Errorf("calls=%d dispatched=%v", h.tr.calls, h.disp.texts())
	}
	if !h.log.has("audio discarded") {
		t.Error("missing discard log")
	}
}

func TestStopWithoutCaptureIsHarmless(t *testing.T) {
	h := start(t, time.Minute)
	h.send('x')
	h.quit(t)
	if h.tr.calls != 0 || !h.log.has("No audio captured") {
		t.Errorf("calls=%d", h.tr.calls)
	}
}

func TestStartWhileCapturingIsIgnored(t *testing.T) {
	h := start(t, time.Minute)
	h.send('c', 'c')
	h.quit(t)
	if h.cap.starts != 1 || !h.log.has("Already capturing") {
		t.Errorf("starts=%d", h.cap.starts)
	}
}

func TestUnknownAndIgnoredCommands(t *testing.T) {
	h := start(t, time.Minute)
	h.send('?', 'i')
	h.quit(t)
	if !h.log.has("Unknown command: '?'") {
		t.Error("unknown command not logged")
	}
	if h.log.has("Unknown command: 'i'") {
		t.Error("'i' must be ignored")
	}
}

func TestQuitDiscardsCaptureAndCancels(t *testing.T) {
	h := start(t, time.Minute)
	h.send('c')
	h.quit(t)

	if h.cap.IsCapturing() || h.cap.discards != 1 {
		t.Errorf("capturing=%v discards=%d", h.cap.IsCapturing(), h.cap.discards)
	}
	if h.ctx.Err() == nil {
		t.Error("app context not cancelled on quit")
	}
}

func TestEscQuits(t *testing.T) {
	h := start(t, time.Minute)
	h.send(keyEsc)
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("Esc did not quit")
	}
}

func TestStartFailureIsLoggedNotFatal(t *testing.T) {
	h := start(t, time.Minute)
	h.cap.startErr = errors.New("no mic")
	h.send('v')
	h.quit(t)
	if !h.log.has("Failed to start capture: no mic") {
		t.Error("start failure not logged")
	}
}

func TestTranscriptionErrorAndEmptyText(t *testing.T) {
	h := start(t, time.Minute)
	h.tr.err = errors.New("model exploded")
	h.send('c', 'x')
	h.quit(t)
	if !h.log.has("Transcription failed: model exploded") || len(h.disp.texts()) != 0 {
		t.Error("error path wrong")
	}

	h = start(t, time.Minute)
	h.tr.text = "  \n"
	h.send('c', 'x')
	h.quit(t)
	if len(h.disp.texts()) != 0 || !h.log.has("No text transcribed") {
		t.Error("blank text must not be dispatched")
	}
}

func TestAlertToggledAroundCommands(t *testing.T) {
	h := start(t, time.Minute)
	h.send('c')
	h.quit(t)
	if !h.log.has("ALERT on") {
		t.Error("alert not shown while capturing")
	}
}

func TestTimeoutAutoStopsAndDiscards(t *testing.T) {
	h := start(t, 50*time.Millisecond)
	h.send('c')
	waitFor(t, h.cap.IsCapturing)

	waitFor(t, func() bool { return !h.cap.IsCapturing() })
	h.quit(t)

	if !h.log.has("Auto-stopping capture") {
		t.Error("missing auto-stop warning")
	}
	if transcribeOnTimeout == false && h.tr.calls != 0 {
		t.Error("timeout path must discard when transcribeOnTimeout is false")
	}
}

func TestManualStopCancelsTimeout(t *testing.T) {
	h := start(t, 60*time.Millisecond)
	h.send('c', 'z')
	time.Sleep(150 * time.Millisecond)
	h.quit(t)

	if h.log.has("Auto-stopping") {
		t.Error("timeout fired after manual stop")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			t.Fatal("condition not met in time")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
