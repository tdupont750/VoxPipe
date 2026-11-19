package input

import "golang.org/x/sys/unix"

// makeCbreak puts the terminal in single-keypress mode: no line buffering and no echo, but
// output processing (so "\n" still returns to column 0) and signal keys (Ctrl-C raises SIGINT)
// stay on. Unlike term.MakeRaw, which clears OPOST and staircases every later line of output.
func makeCbreak(fd int) (restore func(), err error) {
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}

	t := *old
	t.Lflag &^= unix.ICANON | unix.ECHO
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &t); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, old) }, nil
}
