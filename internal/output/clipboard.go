package output

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type clipboardTool struct {
	name string
	args []string
}

// Tried in order; the first one that is installed and succeeds wins.
var clipboardTools = []clipboardTool{
	{"wl-copy", nil},
	{"xclip", []string{"-selection", "clipboard"}},
	{"xsel", []string{"--clipboard", "--input"}},
}

// Clipboard copies text using wl-copy, xclip or xsel.
type Clipboard struct {
	run      Runner
	log      Logger
	lookPath func(string) (string, error)
}

func NewClipboard(run Runner, log Logger) *Clipboard {
	return &Clipboard{run: run, log: log, lookPath: exec.LookPath}
}

func (*Clipboard) Name() string { return "clipboard" }

func (c *Clipboard) Publish(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		c.log.Warn("Empty transcription; skipping clipboard write.")
		return nil
	}

	var errs []error
	for _, tool := range clipboardTools {
		if _, err := c.lookPath(tool.name); err != nil {
			continue
		}
		if _, err := c.run(ctx, Command{Name: tool.name, Args: tool.args, Stdin: text}); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", tool.name, err))
			continue
		}
		c.log.Info("Transcription copied to clipboard.")
		return nil
	}

	if len(errs) == 0 {
		return errors.New("no clipboard tool found (install wl-clipboard, xclip or xsel)")
	}
	return errors.Join(errs...)
}
