package audio

import (
	"fmt"
	"strings"
	"sync"
)

// Logger is the subset of logging.Logger the audio package uses.
type Logger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

// Resolver picks a capture device index: by name substring first, then by configured index.
// An index of -1 means the system default device.
type Resolver struct {
	name     string
	fallback int
	log      Logger

	mu        sync.Mutex
	lastIndex *int
}

// NewResolver builds a Resolver. fallback is the configured deviceIndex (-1 when unset).
func NewResolver(deviceName string, fallback int, log Logger) *Resolver {
	return &Resolver{name: deviceName, fallback: fallback, log: log}
}

// ResolveFrom returns the device index for the given enumerated device names.
func (r *Resolver) ResolveFrom(devices []string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if strings.TrimSpace(r.name) != "" {
		needle := strings.ToLower(r.name)
		for i, d := range devices {
			if !strings.Contains(strings.ToLower(d), needle) {
				continue
			}
			switch {
			case r.lastIndex == nil:
				r.log.Info(fmt.Sprintf("Device with name '%s' found at index %d.", r.name, i))
			case i != *r.lastIndex:
				r.log.Warn(fmt.Sprintf("Device with name '%s' index changed from %d to %d.", r.name, *r.lastIndex, i))
			}
			r.lastIndex = &i
			return i
		}

		switch {
		case r.lastIndex == nil:
			r.log.Warn(fmt.Sprintf("Device with name '%s' NOT found, falling back to index %d.", r.name, r.fallback))
		case r.fallback != *r.lastIndex:
			r.log.Warn(fmt.Sprintf("Device with name '%s' no longer found; index changed from %d to fallback %d.", r.name, *r.lastIndex, r.fallback))
		}
	}

	fb := r.fallback
	r.lastIndex = &fb
	return fb
}

// LogAvailable logs the devices and the one that would be used.
func (r *Resolver) LogAvailable(devices []string) {
	index := r.ResolveFrom(devices)

	r.log.Debug("Available input devices:")
	for i, d := range devices {
		r.log.Debug(fmt.Sprintf("  [%d] %s", i, d))
	}

	name := "System default"
	if index >= 0 && index < len(devices) {
		name = devices[index]
	}
	r.log.Debug(fmt.Sprintf("Using device index %d -> %s", index, name))
}
