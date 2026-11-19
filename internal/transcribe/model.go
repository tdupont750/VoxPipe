// Package transcribe downloads Whisper models and runs inference via whisper.cpp.
package transcribe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gowhisper/internal/config"
)

const defaultModelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

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
	log     Logger
	client  *http.Client
	baseURL string
}

func NewModelEnsurer(log Logger) *ModelEnsurer {
	return &ModelEnsurer{log: log, client: &http.Client{}, baseURL: defaultModelBaseURL}
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
	if err := e.download(ctx, e.baseURL+remoteFileName(kind), path); err != nil {
		return "", err
	}
	e.log.Info("Model downloaded.")
	return path, nil
}

// download streams url to a temp file next to dest, then renames it, so a partial download
// is never mistaken for a complete model.
func (e *ModelEnsurer) download(ctx context.Context, url, dest string) (err error) {
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

	pr := &progressReader{r: resp.Body, total: resp.ContentLength, log: e.log, every: 10 * time.Second}
	if _, err = io.Copy(tmp, pr); err != nil {
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
