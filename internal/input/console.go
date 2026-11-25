package input

import (
	"bufio"
	"context"
	"io"
	"os"
	"time"
	"unicode"

	"golang.org/x/term"
)

const (
	keyEsc       = '\x1b'
	keyCtrlC     = '\x03'
	escSeqWindow = 25 * time.Millisecond
)

// Console reads single keypresses from stdin. On a terminal it switches to cbreak mode so no
// Enter is needed, and restores the terminal on exit. On a pipe it reads characters as they come.
type Console struct {
	in io.Reader
	fd int
}

func NewConsole() *Console { return &Console{in: os.Stdin, fd: int(os.Stdin.Fd())} }

func (c *Console) Run(ctx context.Context, out chan<- rune) error {
	if term.IsTerminal(c.fd) {
		restore, err := makeCbreak(c.fd)
		if err != nil {
			return err
		}
		defer restore()
	}
	readKeys(ctx, c.in, out, escSeqWindow)
	return nil
}

// readKeys turns a byte stream into commands. Newlines are ignored, Ctrl-C becomes 'q', a lone
// Esc is passed through (quit), and escape sequences (arrow keys etc.) are swallowed whole.
// It returns on EOF so an absent stdin does not stop the other sources.
func readKeys(ctx context.Context, r io.Reader, out chan<- rune, escWindow time.Duration) {
	runes := make(chan rune)
	go func() { // exits on EOF or when the process ends; a blocked Read cannot be cancelled
		br := bufio.NewReader(r)
		defer close(runes)
		for {
			ch, _, err := br.ReadRune()
			if err != nil {
				return
			}
			select {
			case runes <- ch:
			case <-ctx.Done():
				return
			}
		}
	}()

	emit := func(ch rune) bool {
		select {
		case out <- ch:
			return true
		case <-ctx.Done():
			return false
		}
	}

	for {
		var ch rune
		var ok bool
		select {
		case <-ctx.Done():
			return
		case ch, ok = <-runes:
			if !ok {
				return
			}
		}

		switch {
		case ch == '\n' || ch == '\r':
			continue
		case ch == keyCtrlC:
			ch = 'q'
		case ch == keyEsc:
			if consumeEscapeSequence(ctx, runes, escWindow) {
				continue
			}
		case unicode.IsControl(ch):
			continue
		}

		if !emit(ch) {
			return
		}
	}
}

// consumeEscapeSequence is called after an Esc. If more input follows within the window it was
// the start of a sequence: swallow it and return true. Otherwise it was a lone Esc.
func consumeEscapeSequence(ctx context.Context, runes <-chan rune, window time.Duration) bool {
	next := func(d time.Duration) (rune, bool) {
		select {
		case ch, ok := <-runes:
			return ch, ok
		case <-time.After(d):
			return 0, false
		case <-ctx.Done():
			return 0, false
		}
	}

	first, ok := next(window)
	if !ok {
		return false
	}

	switch first {
	case '[': // CSI: parameters/intermediates, then a final byte in '@'..'~'
		for {
			ch, ok := next(window)
			if !ok || (ch >= '@' && ch <= '~') {
				return true
			}
		}
	case 'O': // SS3: exactly one more byte
		next(window)
	}
	return true // Alt+key or SS3: swallowed
}
