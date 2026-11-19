// Package output publishes transcribed text to the configured destinations.
package output

import (
	"context"
	"errors"
	"fmt"
	"io"

	"voxpipe/internal/config"
)

// Logger is the subset of logging.Logger used here.
type Logger interface {
	Info(msg string)
	Warn(msg string)
}

// Publisher delivers text to one destination.
type Publisher interface {
	Name() string
	Publish(ctx context.Context, text string) error
}

// Dispatcher fans text out to every publisher, sequentially.
type Dispatcher struct {
	publishers []Publisher
	log        Logger
}

func NewDispatcher(publishers []Publisher, log Logger) (*Dispatcher, error) {
	if len(publishers) == 0 {
		return nil, errors.New("at least one publisher must be provided")
	}
	return &Dispatcher{publishers: publishers, log: log}, nil
}

// Dispatch publishes to each publisher in order. A failing publisher is logged and does not
// stop the others.
func (d *Dispatcher) Dispatch(ctx context.Context, text string) {
	for _, p := range d.publishers {
		if err := p.Publish(ctx, text); err != nil {
			d.log.Warn(fmt.Sprintf("Output %s failed: %v", p.Name(), err))
		}
	}
}

// Build creates publishers for the configured targets, ignoring duplicates.
func Build(targets []config.OutputTarget, stdout io.Writer, log Logger) ([]Publisher, error) {
	seen := map[config.OutputTarget]bool{}
	var out []Publisher
	for _, t := range targets {
		if seen[t] {
			continue
		}
		seen[t] = true

		switch t {
		case config.OutputConsole:
			out = append(out, NewConsole(stdout))
		case config.OutputClipboard:
			out = append(out, NewClipboard(ExecRunner, log))
		case config.OutputYdotool:
			out = append(out, NewYdotool(ExecRunner, log))
		default:
			return nil, fmt.Errorf("unknown output target %q", t)
		}
	}
	return out, nil
}
