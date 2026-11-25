package transcribe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"

	"github.com/tdupont750/VoxPipe/internal/audio"
	"github.com/tdupont750/VoxPipe/internal/text"
)

// Transcriber runs whisper.cpp inference. The model is loaded once; each call gets its own context.
type Transcriber struct {
	model       whisper.Model
	language    string
	threads     uint
	transformer *text.Transformer
	log         Logger

	// whisper contexts share the model; serialize inference to keep CPU use predictable.
	mu sync.Mutex
}

// NewTranscriber loads the model at modelPath.
func NewTranscriber(modelPath, language string, threads int, transformer *text.Transformer, log Logger) (*Transcriber, error) {
	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, fmt.Errorf("load model %s: %w", modelPath, err)
	}
	return &Transcriber{
		model:       model,
		language:    language,
		threads:     uint(max(threads, 1)),
		transformer: transformer,
		log:         log,
	}, nil
}

// Close releases the model.
func (t *Transcriber) Close() error { return t.model.Close() }

// Transcribe converts PCM16 16 kHz mono samples to transformed text. Empty audio yields "".
func (t *Transcriber) Transcribe(ctx context.Context, samples []int16) (string, error) {
	if len(samples) == 0 {
		return "", nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	start := time.Now()

	wctx, err := t.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("new whisper context: %w", err)
	}
	if err := wctx.SetLanguage(t.language); err != nil {
		return "", fmt.Errorf("set language %q: %w", t.language, err)
	}
	wctx.SetThreads(t.threads)

	// No segment callback: passing one forces whisper.cpp into single-segment mode.
	// Returning false from the encoder-begin callback aborts inference on cancellation.
	err = wctx.Process(audio.PCM16ToFloat32(samples), func() bool { return ctx.Err() == nil }, nil, nil)
	if err != nil {
		return "", fmt.Errorf("whisper process: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var parts []string
	for {
		seg, err := wctx.NextSegment()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read segment: %w", err)
		}
		if s := strings.TrimSpace(seg.Text); s != "" {
			parts = append(parts, s)
		}
	}

	raw := strings.TrimSpace(strings.Join(parts, " "))

	elapsed := time.Since(start)
	audioSec := float64(len(samples)) / audio.SampleRate
	t.log.Info(fmt.Sprintf("Transcribed %.0f ms of audio in %.0f ms (%.2fx realtime).",
		audioSec*1000, float64(elapsed.Microseconds())/1000, audioSec/elapsed.Seconds()))

	// Transforms run here so callers stay decoupled; a failing rule is skipped and logged.
	out, err := t.transformer.Transform(raw)
	if err != nil {
		t.log.Warn("Text transform: " + err.Error())
	}
	return out, nil
}
