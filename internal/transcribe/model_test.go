package transcribe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voxpipe/internal/config"
)

type nopLog struct{}

func (nopLog) Debug(string) {}
func (nopLog) Info(string)  {}
func (nopLog) Warn(string)  {}

func TestModelFileNames(t *testing.T) {
	want := map[config.ModelKind]string{
		config.ModelTiny: "ggml-tiny.bin", config.ModelBase: "ggml-base.bin",
		config.ModelSmall: "ggml-small.bin", config.ModelMedium: "ggml-medium.bin",
		config.ModelLarge: "ggml-large.bin",
	}
	for k, w := range want {
		if got := ModelFileName(k); got != w {
			t.Errorf("%s: got %s, want %s", k, got, w)
		}
	}
	if remoteFileName(config.ModelLarge) != "ggml-large-v3-turbo.bin" {
		t.Error("large must download the v3-turbo file")
	}
}

func TestEnsureUsesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ggml-base.bin")
	os.WriteFile(path, []byte("x"), 0o644)

	e := NewModelEnsurer(nopLog{})
	e.baseURL = "http://127.0.0.1:1/" // must not be contacted
	got, err := e.Ensure(context.Background(), config.ModelBase, dir)
	if err != nil || got != path {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestEnsureDownloadsMissingModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ggml-tiny.bin" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(strings.Repeat("m", 1000)))
	}))
	defer srv.Close()

	dir := t.TempDir()
	e := NewModelEnsurer(nopLog{})
	e.baseURL = srv.URL + "/"

	path, err := e.Ensure(context.Background(), config.ModelTiny, dir)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(path); err != nil || st.Size() != 1000 {
		t.Errorf("model file wrong: %v %v", st, err)
	}
	if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) != 0 {
		t.Errorf("temp file left behind: %v", parts)
	}
}

func TestEnsureFailedDownloadLeavesNoFile(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	dir := t.TempDir()
	e := NewModelEnsurer(nopLog{})
	e.baseURL = srv.URL + "/"

	if _, err := e.Ensure(context.Background(), config.ModelBase, dir); err == nil {
		t.Fatal("expected error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("files left behind: %v", entries)
	}
}

func TestEnsureRejectsBadDir(t *testing.T) {
	e := NewModelEnsurer(nopLog{})
	if _, err := e.Ensure(context.Background(), config.ModelBase, ""); err == nil {
		t.Error("empty dir must fail")
	}
	if _, err := e.Ensure(context.Background(), config.ModelBase, "/nonexistent/dir"); err == nil {
		t.Error("missing dir must fail")
	}
}
