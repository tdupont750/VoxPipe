package transcribe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdupont750/VoxPipe/internal/config"
)

type nopLog struct{}

func (nopLog) Debug(string) {}
func (nopLog) Info(string)  {}
func (nopLog) Warn(string)  {}

// modelServer serves body at the remote file name for kind, and nothing else.
func modelServer(t *testing.T, kind config.ModelKind, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+remoteFileName(kind) {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestModelChecksumsPinEveryKind(t *testing.T) {
	for _, k := range []config.ModelKind{
		config.ModelTiny, config.ModelBase, config.ModelSmall, config.ModelMedium, config.ModelLarge,
	} {
		sum, ok := modelChecksums[k]
		if !ok {
			t.Errorf("%s: no pinned checksum", k)
			continue
		}
		if _, err := hex.DecodeString(sum); err != nil || len(sum) != 64 {
			t.Errorf("%s: %q is not a sha256 hex digest", k, sum)
		}
	}
}

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
	body := strings.Repeat("m", 1000)
	srv := modelServer(t, config.ModelTiny, body)

	dir := t.TempDir()
	e := NewModelEnsurer(nopLog{})
	e.baseURL = srv.URL + "/"
	e.checksums = map[config.ModelKind]string{config.ModelTiny: sha256Hex(body)}

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

func TestEnsureRejectsChecksumMismatch(t *testing.T) {
	srv := modelServer(t, config.ModelTiny, "tampered")

	dir := t.TempDir()
	e := NewModelEnsurer(nopLog{})
	e.baseURL = srv.URL + "/"
	e.checksums = map[config.ModelKind]string{config.ModelTiny: sha256Hex("genuine")}

	_, err := e.Ensure(context.Background(), config.ModelTiny, dir)
	if err == nil {
		t.Fatal("a checksum mismatch must fail the download")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error should name the checksum: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("unverified download left behind: %v", entries)
	}
}

func TestEnsureDownloadsUnpinnedModelUnverified(t *testing.T) {
	srv := modelServer(t, config.ModelTiny, "anything")

	dir := t.TempDir()
	e := NewModelEnsurer(nopLog{})
	e.baseURL = srv.URL + "/"
	e.checksums = nil // no pin: download proceeds, with a warning

	if _, err := e.Ensure(context.Background(), config.ModelTiny, dir); err != nil {
		t.Fatalf("unpinned model should still download: %v", err)
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
