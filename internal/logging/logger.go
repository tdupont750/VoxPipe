// Package logging provides the color-coded console logger.
package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	ansiReset      = "\x1b[0m"
	ansiGray       = "\x1b[90m"
	ansiBlue       = "\x1b[34m"
	ansiYellow     = "\x1b[33m"
	ansiRed        = "\x1b[31m"
	ansiRedBg      = "\x1b[41m"
	ansiDefaultBg  = "\x1b[49m"
	carriageReturn = "\r"
)

// Logger writes level-tagged, color-coded lines. Info and Debug go to out, Warn and Error to errOut.
type Logger struct {
	DebugEnabled bool

	mu     sync.Mutex
	out    io.Writer
	errOut io.Writer
	color  bool
}

// New returns a Logger writing to the given writers. Color is always on; use NewStd for TTY detection.
func New(out, errOut io.Writer) *Logger {
	return &Logger{DebugEnabled: true, out: out, errOut: errOut, color: true}
}

// NewStd returns a Logger on stdout/stderr, with color disabled when NO_COLOR is set.
func NewStd() *Logger {
	l := New(os.Stdout, os.Stderr)
	l.color = os.Getenv("NO_COLOR") == ""
	return l
}

func (l *Logger) Debug(msg string) {
	if !l.DebugEnabled {
		return
	}
	l.writeLevel("DEBUG", msg, false, ansiGray)
}

func (l *Logger) Info(msg string)  { l.writeLevel("INFO", msg, false, ansiBlue) }
func (l *Logger) Warn(msg string)  { l.writeLevel("WARN", msg, true, ansiYellow) }
func (l *Logger) Error(msg string) { l.writeLevel("ERROR", msg, true, ansiRed) }

func (l *Logger) Debugf(format string, a ...any) { l.Debug(fmt.Sprintf(format, a...)) }
func (l *Logger) Infof(format string, a ...any)  { l.Info(fmt.Sprintf(format, a...)) }
func (l *Logger) Warnf(format string, a ...any)  { l.Warn(fmt.Sprintf(format, a...)) }
func (l *Logger) Errorf(format string, a ...any) { l.Error(fmt.Sprintf(format, a...)) }

// ErrorErr logs err untagged, like the C# Error(Exception) overload.
func (l *Logger) ErrorErr(err error) {
	l.writeLine(true, err.Error(), ansiRed)
}

// ToggleAlert shows alertText on a red background, or erases it from the start of the current line.
func (l *Logger) ToggleAlert(alertText string, show bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if show {
		if l.color {
			fmt.Fprint(l.out, ansiRedBg+alertText+ansiDefaultBg)
		} else {
			fmt.Fprint(l.out, alertText)
		}
		return
	}

	blank := strings.Repeat(" ", len([]rune(alertText)))
	fmt.Fprint(l.out, carriageReturn+blank+carriageReturn)
}

func (l *Logger) writeLevel(level, msg string, isErr bool, color string) {
	// Avoid double-tagging if the message already starts with [LEVEL]
	if !hasPrefixFold(msg, "["+level+"]") {
		msg = fmt.Sprintf("[APP] [%s] %s", level, msg)
	}
	l.writeLine(isErr, msg, color)
}

func (l *Logger) writeLine(isErr bool, msg, color string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.out
	if isErr {
		w = l.errOut
	}
	if l.color {
		fmt.Fprintf(w, "%s%s%s\n", color, msg, ansiReset)
	} else {
		fmt.Fprintln(w, msg)
	}
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
