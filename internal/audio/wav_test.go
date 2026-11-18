package audio

import (
	"bytes"
	"encoding/binary"
	"testing"
)

const testRate = 16000

func TestWavWritesCanonicalHeader(t *testing.T) {
	samples := []int16{1, -1, 32767, -32768}
	b := WavFromPCM16(samples, testRate)
	le := binary.LittleEndian
	data := len(samples) * 2

	if len(b) != WavHeaderBytes+data {
		t.Fatalf("len = %d", len(b))
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"RIFF", string(b[0:4]), "RIFF"},
		{"chunk size", le.Uint32(b[4:]), uint32(4 + 24 + 8 + data)},
		{"WAVE", string(b[8:12]), "WAVE"},
		{"fmt", string(b[12:16]), "fmt "},
		{"subchunk1", le.Uint32(b[16:]), uint32(16)},
		{"PCM", le.Uint16(b[20:]), uint16(1)},
		{"mono", le.Uint16(b[22:]), uint16(1)},
		{"rate", le.Uint32(b[24:]), uint32(testRate)},
		{"byte rate", le.Uint32(b[28:]), uint32(testRate * 2)},
		{"block align", le.Uint16(b[32:]), uint16(2)},
		{"bits", le.Uint16(b[34:]), uint16(16)},
		{"data", string(b[36:40]), "data"},
		{"data size", le.Uint32(b[40:]), uint32(data)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestWavWritesSamplesLittleEndian(t *testing.T) {
	samples := []int16{1, -1, 32767, -32768, 0x1234}
	b := WavFromPCM16(samples, testRate)

	for i, s := range samples {
		if got := int16(binary.LittleEndian.Uint16(b[WavHeaderBytes+i*2:])); got != s {
			t.Errorf("sample %d: got %d, want %d", i, got, s)
		}
	}
	if b[WavHeaderBytes+8] != 0x34 || b[WavHeaderBytes+9] != 0x12 {
		t.Error("raw byte order is not little-endian")
	}
}

func TestWavEmptySamplesWritesHeaderOnly(t *testing.T) {
	b := WavFromPCM16(nil, testRate)
	if len(b) != WavHeaderBytes {
		t.Fatalf("len = %d", len(b))
	}
	if binary.LittleEndian.Uint32(b[40:]) != 0 || binary.LittleEndian.Uint32(b[4:]) != 36 {
		t.Error("empty sizes wrong")
	}
}

func TestWavLargeBufferMatchesPerSampleEncoding(t *testing.T) {
	samples := make([]int16, testRate*60)
	expected := make([]byte, 0, len(samples)*2)
	for i := range samples {
		samples[i] = int16(i%1000 - 500)
		expected = append(expected, byte(uint16(samples[i])), byte(uint16(samples[i])>>8))
	}
	if !bytes.Equal(WavFromPCM16(samples, testRate)[WavHeaderBytes:], expected) {
		t.Error("bulk encoding differs from per-sample encoding")
	}
}
