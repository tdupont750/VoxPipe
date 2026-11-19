package output

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	ydotoolSocket = "/tmp/.ydotool_socket"
	// backspaceToken at the start of the text sends a Backspace key press first.
	// Key code 14 is KEY_BACKSPACE in /usr/include/linux/input-event-codes.h.
	backspaceToken = "{ydotool:KEY_BACKSPACE}"
)

// Ydotool types text through the virtual keyboard.
type Ydotool struct {
	run Runner
	log Logger
}

func NewYdotool(run Runner, log Logger) *Ydotool { return &Ydotool{run: run, log: log} }

func (*Ydotool) Name() string { return "ydotool" }

func (y *Ydotool) Publish(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		y.log.Warn("Empty transcription; skipping ydotool write.")
		return nil
	}

	if rest, ok := strings.CutPrefix(text, backspaceToken); ok {
		if err := y.exec(ctx, []string{"key", "14:1", "14:0"}, ""); err != nil {
			return err
		}
		text = rest
		if text == "" {
			return nil
		}
	}

	if err := y.exec(ctx, []string{"type", "--key-delay=3", "--key-hold=2", "--file=-"}, text); err != nil {
		return err
	}
	y.log.Info("Transcription sent via ydotool.")
	return nil
}

func (y *Ydotool) exec(ctx context.Context, args []string, stdin string) error {
	stderr, err := y.run(ctx, Command{
		Name:          "ydotool",
		Args:          args,
		Env:           []string{"YDOTOOL_SOCKET=" + ydotoolSocket},
		Stdin:         stdin,
		CaptureStderr: true,
	})
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("ydotool exited with code %d: %s", exitErr.ExitCode(), stderr)
	}
	return fmt.Errorf("run ydotool: %w", err)
}
