// Package app is the command loop that ties capture, transcription and output together.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"voxpipe/internal/audio"
)

const (
	captureAlert = "--- CAPTURING ---"
	keyEsc       = '\x1b'

	// transcribeOnTimeout mirrors the C# app, which passed discardTranscribe=true on auto-stop,
	// so audio captured up to the timeout is dropped. Flip to true to transcribe it instead.
	transcribeOnTimeout = false
)

// Capturer records audio. audio.Capture implements it.
type Capturer interface {
	Start() error
	StopAndGet() ([]int16, error)
	StopAndDiscard()
	IsCapturing() bool
}

// Transcriber converts audio to text.
type Transcriber interface {
	Transcribe(ctx context.Context, samples []int16) (string, error)
}

// Dispatcher delivers text to the configured outputs.
type Dispatcher interface {
	Dispatch(ctx context.Context, text string)
}

// Logger is the subset of logging.Logger used here.
type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	ToggleAlert(alertText string, show bool)
}

var _ Capturer = (*audio.Capture)(nil)

// App processes commands one at a time.
type App struct {
	capture     Capturer
	transcriber Transcriber
	dispatcher  Dispatcher
	log         Logger
	timeout     time.Duration

	timeoutCh chan struct{}
}

func New(capture Capturer, transcriber Transcriber, dispatcher Dispatcher, captureTimeout time.Duration, log Logger) *App {
	return &App{
		capture:     capture,
		transcriber: transcriber,
		dispatcher:  dispatcher,
		log:         log,
		timeout:     captureTimeout,
		timeoutCh:   make(chan struct{}, 1),
	}
}

// Run handles commands until ctx is cancelled or a quit command arrives, in which case cancel
// is called so the other goroutines stop. Commands, timeouts and transcription all run on this
// goroutine, so capture state needs no extra locking here.
func (a *App) Run(ctx context.Context, cancel context.CancelFunc, cmds <-chan rune) {
	timer := audio.NewTimeoutManager(ctx, a.timeout, func() {
		select {
		case a.timeoutCh <- struct{}{}:
		default:
		}
	})
	defer timer.Cancel()

	restart := func() { timer.Restart(); a.drainTimeout() }
	stop := func() { timer.Cancel(); a.drainTimeout() }

	for {
		select {
		case <-ctx.Done():
			a.capture.StopAndDiscard()
			return

		case <-a.timeoutCh:
			if !a.capture.IsCapturing() {
				continue // stale: capture already stopped
			}
			a.toggleAlert(false)
			a.log.Warn(fmt.Sprintf("Auto-stopping capture after %d seconds...", int(a.timeout.Seconds())))
			a.stopAndTranscribe(ctx, !transcribeOnTimeout)

		case raw := <-cmds:
			a.toggleAlert(false)
			cmd := unicode.ToLower(raw)

			if cmd == 'q' || cmd == keyEsc {
				a.log.Debug("Stopping...")
				stop()
				a.capture.StopAndDiscard()
				cancel()
				return
			}

			switch cmd {
			case 'v':
				if a.capture.IsCapturing() {
					stop()
					a.stopAndTranscribe(ctx, false)
				} else {
					a.startCapture()
					restart()
				}
			case 'c':
				a.startCapture()
				restart()
			case 'z':
				stop()
				a.capture.StopAndDiscard()
				a.log.Info("Capture stopped; audio discarded.")
			case 'x':
				stop()
				a.stopAndTranscribe(ctx, false)
			case 'i':
				// legacy sentinel, no-op
			default:
				a.log.Error(fmt.Sprintf("Unknown command: '%c'", cmd))
			}

			a.toggleAlert(true)
		}
	}
}

func (a *App) drainTimeout() {
	select {
	case <-a.timeoutCh:
	default:
	}
}

func (a *App) toggleAlert(show bool) {
	if a.capture.IsCapturing() {
		a.log.ToggleAlert(captureAlert, show)
	}
}

func (a *App) startCapture() {
	if a.capture.IsCapturing() {
		a.log.Info("Already capturing; ignoring 'c'/'v start'.")
		return
	}

	a.log.Info("Starting capture...")
	if err := a.capture.Start(); err != nil {
		a.log.Error("Failed to start capture: " + err.Error())
	}
}

func (a *App) stopAndTranscribe(ctx context.Context, discard bool) {
	a.log.Info("Stopping capture and transcribing...")
	samples, err := a.capture.StopAndGet()
	if err != nil {
		a.log.Warn("Stopping capture: " + err.Error())
	}

	if discard || len(samples) == 0 {
		a.log.Info("No audio captured to transcribe.")
		return
	}

	text, err := a.transcriber.Transcribe(ctx, samples)
	switch {
	case errors.Is(err, context.Canceled):
		a.log.Info("Transcription cancelled.")
		return
	case err != nil:
		a.log.Error("Transcription failed: " + err.Error())
		return
	}

	if strings.TrimSpace(text) == "" {
		a.log.Debug("No text transcribed.")
		return
	}
	a.dispatcher.Dispatch(ctx, text)
}
