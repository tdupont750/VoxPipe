package audio

import "encoding/binary"

// SampleRate is the capture rate Whisper expects.
const SampleRate = 16000

// sampleBuffer accumulates PCM16 samples. It is not safe for concurrent use.
type sampleBuffer struct {
	samples  []int16
	capacity int
}

// newSampleBuffer sizes the buffer up front so it never has to grow and copy mid-capture.
// Seconds is clamped so a very large (or absent) timeout still reserves something sane.
func newSampleBuffer(timeoutSeconds int) *sampleBuffer {
	seconds := timeoutSeconds
	if seconds <= 0 {
		seconds = 60
	}
	seconds = min(max(seconds, 1), 300)

	c := SampleRate * seconds
	return &sampleBuffer{samples: make([]int16, 0, c), capacity: c}
}

// appendPCM16 appends little-endian 16-bit samples from raw bytes.
func (b *sampleBuffer) appendPCM16(raw []byte) {
	for i := 0; i+1 < len(raw); i += 2 {
		b.samples = append(b.samples, int16(binary.LittleEndian.Uint16(raw[i:])))
	}
}

// take returns a copy of the samples and recycles the backing array. A capture that ran far
// past the expected length is dropped instead, so one long session cannot pin its peak forever.
func (b *sampleBuffer) take() []int16 {
	out := make([]int16, len(b.samples))
	copy(out, b.samples)
	b.reset()
	return out
}

func (b *sampleBuffer) reset() {
	if cap(b.samples) > b.capacity*4 {
		b.samples = make([]int16, 0, b.capacity)
		return
	}
	b.samples = b.samples[:0]
}

// PCM16ToFloat32 converts PCM16 samples to the [-1, 1) float32 range whisper.cpp expects.
func PCM16ToFloat32(samples []int16) []float32 {
	out := make([]float32, len(samples))
	for i, s := range samples {
		out[i] = float32(s) / 32768.0
	}
	return out
}
