package input

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type nopLog struct{}

func (nopLog) Info(string) {}
func (nopLog) Warn(string) {}

func collect(t *testing.T, in string) string {
	t.Helper()
	out := make(chan rune, 64)
	if err := readKeys(context.Background(), strings.NewReader(in), out, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	close(out)
	var sb strings.Builder
	for r := range out {
		sb.WriteRune(r)
	}
	return sb.String()
}

func TestReadKeysPlainCharacters(t *testing.T) {
	if got := collect(t, "vcxzq"); got != "vcxzq" {
		t.Errorf("got %q", got)
	}
}

func TestReadKeysIgnoresNewlinesAndControl(t *testing.T) {
	if got := collect(t, "v\r\nc\x01\x00x"); got != "vcx" {
		t.Errorf("got %q", got)
	}
}

func TestReadKeysCtrlCQuits(t *testing.T) {
	if got := collect(t, "\x03"); got != "q" {
		t.Errorf("got %q", got)
	}
}

func TestReadKeysSwallowsEscapeSequences(t *testing.T) {
	// up arrow, F5 (CSI with params), SS3 F1, Alt+q, then a real command
	if got := collect(t, "\x1b[A\x1b[15~\x1bOP\x1bqv"); got != "v" {
		t.Errorf("got %q", got)
	}
}

func TestReadKeysLoneEscIsPassedThrough(t *testing.T) {
	pr, pw := io.Pipe()
	out := make(chan rune, 4)
	go readKeys(context.Background(), pr, out, 20*time.Millisecond)

	pw.Write([]byte{0x1b}) // nothing follows within the window
	select {
	case r := <-out:
		if r != 0x1b {
			t.Errorf("got %q", r)
		}
	case <-time.After(time.Second):
		t.Fatal("lone Esc was not emitted")
	}
}

func TestReadKeysUTF8(t *testing.T) {
	if got := collect(t, "é"); got != "é" {
		t.Errorf("got %q", got)
	}
}

func TestReadKeysStopsOnCancel(t *testing.T) {
	pr, _ := io.Pipe() // never delivers
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- readKeys(ctx, pr, make(chan rune), time.Millisecond) }()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("readKeys did not return after cancel")
	}
}

func TestHTTPHandler(t *testing.T) {
	out := make(chan rune, 4)
	h := Handler(context.Background(), out)

	tests := []struct {
		method, path string
		status       int
		cmd          rune
	}{
		{"POST", "/command/v", 200, 'v'},
		{"POST", "/command/q/", 200, 'q'},
		{"GET", "/command/v", 400, 0},
		{"POST", "/command/vv", 400, 0},
		{"POST", "/command/", 400, 0},
		{"POST", "/other/v", 400, 0},
		{"POST", "/command/v/extra", 400, 0},
		{"POST", "/", 400, 0},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.status {
			t.Errorf("%s %s: status %d, want %d", tt.method, tt.path, rec.Code, tt.status)
		}
		if tt.cmd != 0 {
			if got := <-out; got != tt.cmd {
				t.Errorf("%s %s: cmd %q, want %q", tt.method, tt.path, got, tt.cmd)
			}
		}
	}
	if len(out) != 0 {
		t.Errorf("rejected requests produced commands: %d", len(out))
	}
}

func TestHTTPHandlerUnavailableWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	Handler(ctx, make(chan rune)).ServeHTTP(rec, httptest.NewRequest("POST", "/command/v", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d", rec.Code)
	}
}

func TestHTTPRunServesAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan rune, 1)
	srv := NewHTTP(freePort(t), nopLog{})
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx, out) }()

	url := "http://localhost:" + itoa(srv.port) + "/command/v"
	var resp *http.Response
	var err error
	for range 50 {
		if resp, err = http.Post(url, "", nil); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || <-out != 'v' {
		t.Errorf("status %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}

type funcSource func(ctx context.Context, out chan<- rune) error

func (f funcSource) Run(ctx context.Context, out chan<- rune) error { return f(ctx, out) }

func TestFanInMergesSources(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	send := func(r rune) Source {
		return funcSource(func(ctx context.Context, out chan<- rune) error {
			out <- r
			<-ctx.Done()
			return nil
		})
	}

	ch, wait := FanIn(ctx, []Source{send('a'), send('b')}, nopLog{})
	got := map[rune]bool{<-ch: true, <-ch: true}
	if !got['a'] || !got['b'] {
		t.Errorf("got %v", got)
	}

	cancel()
	done := make(chan struct{})
	go func() { wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("wait did not return")
	}
}
