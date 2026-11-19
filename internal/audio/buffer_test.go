package audio

import "testing"

func TestBufferAppendAndTake(t *testing.T) {
	b := newSampleBuffer(1)
	b.appendPCM16([]byte{0x01, 0x00, 0xff, 0xff, 0x34, 0x12})

	got := b.take()
	want := []int16{1, -1, 0x1234}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %d, want %d", i, got[i], want[i])
		}
	}
	if len(b.samples) != 0 {
		t.Error("buffer not reset after take")
	}
}

func TestBufferTakeIsACopy(t *testing.T) {
	b := newSampleBuffer(1)
	b.appendPCM16([]byte{0x05, 0x00})
	got := b.take()

	b.appendPCM16([]byte{0x09, 0x00})
	if got[0] != 5 {
		t.Errorf("taken slice aliased the reused buffer: %d", got[0])
	}
}

func TestBufferIgnoresTrailingOddByte(t *testing.T) {
	b := newSampleBuffer(1)
	b.appendPCM16([]byte{0x01, 0x00, 0x02})
	if len(b.samples) != 1 {
		t.Errorf("len = %d", len(b.samples))
	}
}

func TestBufferCapacityClamped(t *testing.T) {
	cases := map[int]int{0: 60, -5: 60, 1: 1, 10: 10, 9999: 300}
	for secs, want := range cases {
		if got := newSampleBuffer(secs).capacity; got != SampleRate*want {
			t.Errorf("timeout %d: capacity %d, want %d", secs, got, SampleRate*want)
		}
	}
}

func TestBufferDropsOversizedBackingArray(t *testing.T) {
	b := newSampleBuffer(1)
	b.samples = make([]int16, 0, b.capacity*5)
	b.reset()
	if cap(b.samples) != b.capacity {
		t.Errorf("cap = %d, want %d", cap(b.samples), b.capacity)
	}
}

func TestPCM16ToFloat32(t *testing.T) {
	got := PCM16ToFloat32([]int16{0, 16384, -32768, 32767})
	want := []float32{0, 0.5, -1, 32767.0 / 32768.0}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
