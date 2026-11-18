package audio

import (
	"context"
	"errors"
	"sync"
	"time"
)

// TimeoutManager runs a callback if a countdown elapses before it is cancelled.
// Restart and Cancel are safe to call repeatedly and from any goroutine.
type TimeoutManager struct {
	parent    context.Context
	timeout   time.Duration
	onTimeout func()

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewTimeoutManager returns a manager that fires onTimeout after timeout. A timeout <= 0 disables it.
// Cancelling parent also cancels any pending countdown without firing the callback.
func NewTimeoutManager(parent context.Context, timeout time.Duration, onTimeout func()) *TimeoutManager {
	return &TimeoutManager{parent: parent, timeout: timeout, onTimeout: onTimeout}
}

// Restart cancels any active countdown and starts a new one.
func (t *TimeoutManager) Restart() {
	t.Cancel()
	if t.timeout <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(t.parent, t.timeout)
	t.mu.Lock()
	t.cancel = cancel
	t.mu.Unlock()

	go func() {
		<-ctx.Done()
		cancel() // release resources; harmless after expiry
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.onTimeout()
		}
	}()
}

// Cancel stops the active countdown, if any, without firing the callback.
func (t *TimeoutManager) Cancel() {
	t.mu.Lock()
	cancel := t.cancel
	t.cancel = nil
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}
