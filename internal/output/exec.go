package output

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Command describes an external process to run.
type Command struct {
	Name string
	Args []string
	Env  []string // extra KEY=VALUE entries added to the inherited environment
	// Stdin is piped to the process; empty means no input.
	Stdin string
	// CaptureStderr collects stderr for the returned string. Leave false for tools that fork
	// a background daemon (wl-copy, xclip), which would keep the pipe open.
	CaptureStderr bool
}

// Runner runs a Command and returns captured stderr. It is injectable for tests.
type Runner func(ctx context.Context, c Command) (stderr string, err error)

// ExecRunner runs the command with os/exec, directly without a shell.
func ExecRunner(ctx context.Context, c Command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Env = append(os.Environ(), c.Env...)
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	var errBuf bytes.Buffer
	if c.CaptureStderr {
		cmd.Stderr = &errBuf
	}
	cmd.WaitDelay = 2 * time.Second

	err := cmd.Run()
	return strings.TrimSpace(errBuf.String()), err
}
