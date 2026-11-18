package audio

import (
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
)

// Capture records mono 16 kHz PCM16 from a microphone via miniaudio.
type Capture struct {
	ctx         *malgo.AllocatedContext
	resolver    *Resolver
	frameLength uint32

	mu        sync.Mutex
	buf       *sampleBuffer
	device    *malgo.Device
	capturing bool
}

// NewCapture initializes the audio backend. Call Close when done.
func NewCapture(resolver *Resolver, frameLength, captureTimeoutSeconds int) (*Capture, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}
	return &Capture{
		ctx:         ctx,
		resolver:    resolver,
		frameLength: uint32(max(frameLength, 0)),
		buf:         newSampleBuffer(captureTimeoutSeconds),
	}, nil
}

// Close stops any capture and releases the audio backend.
func (c *Capture) Close() {
	c.StopAndDiscard()
	_ = c.ctx.Uninit()
	c.ctx.Free()
}

// IsCapturing reports whether a capture is in progress.
func (c *Capture) IsCapturing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capturing
}

// DeviceNames lists the available capture devices, in index order.
func (c *Capture) DeviceNames() ([]string, error) {
	infos, err := c.ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(infos))
	for i := range infos {
		names[i] = infos[i].Name()
	}
	return names, nil
}

// Start begins capturing. It is a no-op if already capturing.
func (c *Capture) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.capturing {
		return nil
	}

	infos, err := c.ctx.Devices(malgo.Capture)
	if err != nil {
		return fmt.Errorf("list capture devices: %w", err)
	}
	names := make([]string, len(infos))
	for i := range infos {
		names[i] = infos[i].Name()
	}
	idx := c.resolver.ResolveFrom(names)

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInFrames = c.frameLength

	switch {
	case idx == -1:
		// system default
	case idx >= 0 && idx < len(infos):
		id := infos[idx].ID
		cfg.Capture.DeviceID = id.Pointer()
	default:
		return fmt.Errorf("device index %d out of range (%d devices)", idx, len(infos))
	}

	c.buf.reset()

	dev, err := malgo.InitDevice(c.ctx.Context, cfg, malgo.DeviceCallbacks{Data: c.onData})
	if err != nil {
		return fmt.Errorf("init capture device: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start capture device: %w", err)
	}

	c.device = dev
	c.capturing = true
	return nil
}

// onData runs on the audio thread.
func (c *Capture) onData(_, in []byte, _ uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.capturing {
		c.buf.appendPCM16(in)
	}
}

// StopAndGet stops capturing and returns the samples. It returns nil if not capturing.
func (c *Capture) StopAndGet() ([]int16, error) {
	return c.stop(true)
}

// StopAndDiscard stops capturing and drops the samples.
func (c *Capture) StopAndDiscard() {
	_, _ = c.stop(false)
}

func (c *Capture) stop(keep bool) ([]int16, error) {
	c.mu.Lock()
	if !c.capturing {
		c.mu.Unlock()
		return nil, nil
	}
	c.capturing = false // callback now drops frames
	dev := c.device
	c.device = nil
	c.mu.Unlock()

	// Stop outside the lock: miniaudio waits for an in-flight callback, which takes the lock.
	var stopErr error
	if err := dev.Stop(); err != nil {
		stopErr = fmt.Errorf("stop capture device: %w", err)
	}
	dev.Uninit()

	c.mu.Lock()
	defer c.mu.Unlock()
	if !keep {
		c.buf.reset()
		return nil, stopErr
	}
	return c.buf.take(), stopErr
}
