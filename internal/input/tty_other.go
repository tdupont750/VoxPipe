//go:build !linux

package input

import "golang.org/x/term"

// makeCbreak falls back to raw mode where termios is not wired up. Output may staircase.
func makeCbreak(fd int) (restore func(), err error) {
	old, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd, old) }, nil
}
