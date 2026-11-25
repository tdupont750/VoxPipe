// Package transcribe downloads Whisper models and runs inference via whisper.cpp.
package transcribe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tdupont750/VoxPipe/internal/config"
)

const defaultModelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

// modelChecksums pins the SHA-256 of each model file published by the whisper.cpp HuggingFace
// repo, so a corrupt or substituted download is never saved as a usable model. The values are
// the LFS object ids HuggingFace serves as x-linked-etag for the files in remoteFileName.
// If a model is ever republished upstream, the download fails until this map is updated.
var modelChecksums = map[config.ModelKind]string{
	config.ModelTiny:   "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21",
	config.ModelBase:   "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe",
	config.ModelSmall:  "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b",
	config.ModelMedium: "6c14d5adee5f86394037b4e4e8b59f1673b6cee10e3cf0b11bbdbee79c156208",
	config.ModelLarge:  "1fc70f774d38eb169993ac391eea357ef47c88757ef72ee5943879b7e8e2bc69",
}

// newDownloadClient returns a client with no overall deadline, since a model is up to ~1.6 GB
// on a slow link, but with per-stage timeouts so a stalled connection cannot hang forever.
func newDownloadClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       30 * time.Second,
		},
	}
}

// Logger is the subset of logging.Logger used here.
type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
}

// ModelFileName is the local file name for a model kind. Names match the C# app so
// existing model directories keep working.
func ModelFileName(kind config.ModelKind) string {
	switch kind {
	case config.ModelTiny:
		return "ggml-tiny.bin"
	case config.ModelSmall:
		return "ggml-small.bin"
	case config.ModelMedium:
		return "ggml-medium.bin"
	case config.ModelLarge:
		return "ggml-large.bin"
	default:
		return "ggml-base.bin"
	}
}

// remoteFileName is the HuggingFace file for a model kind. Large maps to large-v3-turbo,
// as the C# app did (GgmlType.LargeV3Turbo), but is saved under the ggml-large.bin name.
func remoteFileName(kind config.ModelKind) string {
	if kind == config.ModelLarge {
		return "ggml-large-v3-turbo.bin"
	}
	return ModelFileName(kind)
}

// ModelEnsurer makes sure a model file exists locally, downloading it if missing.
type ModelEnsurer struct {
	log       Logger
	client    *http.Client
	baseURL   string
	checksums map[config.ModelKind]string
}

func NewModelEnsurer(log Logger) *ModelEnsurer {
	return &ModelEnsurer{
		log:       log,
		client:    newDownloadClient(),
		baseURL:   defaultModelBaseURL,
		checksums: modelChecksums,
	}
}

// Ensure returns the path of the model file, downloading it first if it does not exist.
func (e *ModelEnsurer) Ensure(ctx context.Context, kind config.ModelKind, modelDir string) (string, error) {
	if modelDir == "" {
		return "", errors.New("model directory must be provided")
	}
	if st, err := os.Stat(modelDir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("model directory does not exist: %s", modelDir)
	}

	path := filepath.Join(modelDir, ModelFileName(kind))
	if _, err := os.Stat(path); err == nil {
		e.log.Debug("Using existing model: " + path)
		return path, nil
	}

	e.log.Warn(fmt.Sprintf("Model '%s' not found. Downloading Whisper %s model...", path, kind))
	want, ok := e.checksums[kind]
	if !ok {
		e.log.Warn(fmt.Sprintf("No pinned checksum for the %s model; the download cannot be verified.", kind))
	}
	if err := e.download(ctx, e.baseURL+remoteFileName(kind), path, want); err != nil {
		return "", err
	}
	e.log.Info("Model downloaded.")
	return path, nil
}

// download streams url to a temp file next to dest, verifies its SHA-256 against wantSHA256
// (skipped when empty), then renames it, so neither a partial nor an unverified download is
// ever mistaken for a complete model.
func (e *ModelEnsurer) download(ctx context.Context, url, dest, wantSHA256 string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("download model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download model: %s returned %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".*.part")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()

	var sum hash.Hash
	dst := io.Writer(tmp)
	if wantSHA256 != "" {
		sum = sha256.New()
		dst = io.MultiWriter(tmp, sum)
	}

	pr := &progressReader{r: resp.Body, total: resp.ContentLength, log: e.log, every: 10 * time.Second}
	if _, err = io.Copy(dst, pr); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("download model: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if resp.ContentLength > 0 && pr.read != resp.ContentLength {
		err = fmt.Errorf("download model: got %d bytes, expected %d", pr.read, resp.ContentLength)
		return err
	}
	if sum != nil {
		if got := hex.EncodeToString(sum.Sum(nil)); !strings.EqualFold(got, wantSHA256) {
			err = fmt.Errorf("download model: %s has sha256 %s, expected %s", url, got, wantSHA256)
			return err
		}
		e.log.Debug("Model checksum verified: " + wantSHA256)
	}
	return os.Rename(tmp.Name(), dest)
}

// progressReader logs download progress at most once per interval.
type progressReader struct {
	r     io.Reader
	total int64
	read  int64
	log   Logger
	every time.Duration
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)

	if now := time.Now(); now.Sub(p.last) >= p.every {
		p.last = now
		if p.total > 0 {
			p.log.Info(fmt.Sprintf("Downloading model: %d%% (%d/%d MB)", p.read*100/p.total, p.read>>20, p.total>>20))
		} else {
			p.log.Info(fmt.Sprintf("Downloading model: %d MB", p.read>>20))
		}
	}
	return n, err
}
