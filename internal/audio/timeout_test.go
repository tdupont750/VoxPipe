package audio

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestTimeoutFires(t *testing.T) {
	fired := make(chan struct{}, 1)
	m := NewTimeoutManager(context.Background(), 20*time.Millisecond, func() { fired <- struct{}{} })
	m.Restart()

	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("timeout callback did not fire")
	}
}

func TestTimeoutCancelPreventsCallback(t *testing.T) {
	var n atomic.Int32
	m := NewTimeoutManager(context.Background(), 20*time.Millisecond, func() { n.Add(1) })
	m.Restart()
	m.Cancel()

	time.Sleep(80 * time.Millisecond)
	if n.Load() != 0 {
		t.Error("callback fired after Cancel")
	}
}

func TestTimeoutRestartFiresOnce(t *testing.T) {
	var n atomic.Int32
	m := NewTimeoutManager(context.Background(), 40*time.Millisecond, func() { n.Add(1) })
	m.Restart()
	time.Sleep(20 * time.Millisecond)
	m.Restart() // resets the countdown; the first one must not fire

	time.Sleep(120 * time.Millisecond)
	if got := n.Load(); got != 1 {
		t.Errorf("callback fired %d times, want 1", got)
	}
}

func TestTimeoutParentCancelSuppressesCallback(t *testing.T) {
	var n atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	m := NewTimeoutManager(ctx, 20*time.Millisecond, func() { n.Add(1) })
	m.Restart()
	cancel()

	time.Sleep(80 * time.Millisecond)
	if n.Load() != 0 {
		t.Error("callback fired after parent cancel")
	}
}

func TestTimeoutDisabledWhenNonPositive(t *testing.T) {
	var n atomic.Int32
	m := NewTimeoutManager(context.Background(), 0, func() { n.Add(1) })
	m.Restart()

	time.Sleep(30 * time.Millisecond)
	if n.Load() != 0 {
		t.Error("disabled timeout fired")
	}
}
