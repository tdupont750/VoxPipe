// Package input produces single-character commands from the console and HTTP.
package input

import (
	"context"
	"sync"
)

// Logger is the subset of logging.Logger used here.
type Logger interface {
	Info(msg string)
	Warn(msg string)
}

// Source produces commands until ctx is cancelled or it fails. Run must return promptly
// after ctx is cancelled and must not close out.
type Source interface {
	Run(ctx context.Context, out chan<- rune) error
}

// FanIn runs every source concurrently into one buffered channel. The returned wait function
// blocks until all sources have returned; cancel ctx first. The channel is never closed.
func FanIn(ctx context.Context, sources []Source, log Logger) (<-chan rune, func()) {
	out := make(chan rune, 16)

	var wg sync.WaitGroup
	for _, s := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Run(ctx, out); err != nil && ctx.Err() == nil {
				log.Warn("Command source error: " + err.Error())
			}
		}()
	}
	return out, wg.Wait
}
