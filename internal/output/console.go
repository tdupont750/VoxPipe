package output

import (
	"context"
	"fmt"
	"io"
)

// Console writes the transcript to stdout, not through the logger.
type Console struct{ w io.Writer }

func NewConsole(w io.Writer) *Console { return &Console{w: w} }

func (*Console) Name() string { return "console" }

func (c *Console) Publish(_ context.Context, text string) error {
	_, err := fmt.Fprintf(c.w, "\n%s\n\n", text)
	return err
}
