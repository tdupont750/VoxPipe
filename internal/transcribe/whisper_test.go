package transcribe

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voxpipe/internal/config"
	"voxpipe/internal/text"
)

// readWavSamples extracts PCM16 samples from the data chunk of a 16 kHz mono WAV.
func readWavSamples(t *testing.T, path string) []int16 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("sample missing: %v", err)
	}
	for i := 12; i+8 <= len(b); {
		size := int(binary.LittleEndian.Uint32(b[i+4:]))
		if string(b[i:i+4]) == "data" {
			data := b[i+8 : min(i+8+size, len(b))]
			out := make([]int16, len(data)/2)
			for j := range out {
				out[j] = int16(binary.LittleEndian.Uint16(data[j*2:]))
			}
			return out
		}
		i += 8 + size
	}
	t.Fatal("no data chunk")
	return nil
}

// TestSmokeJFK transcribes the whisper.cpp JFK sample. Skipped when the model is absent.
func TestSmokeJFK(t *testing.T) {
	model := filepath.Join("..", "..", "models", ModelFileName(config.ModelBase))
	if _, err := os.Stat(model); err != nil {
		t.Skipf("model not present: %v", err)
	}
	samples := readWavSamples(t, filepath.Join("..", "..", "third_party", "whisper.cpp", "samples", "jfk.wav"))

	tr, err := NewTranscriber(model, "en", 4, text.NewTransformer(nil, text.NewReplacer()), nopLog{})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	got, err := tr.Transcribe(context.Background(), samples)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("transcript: %q", got)
	if !strings.Contains(strings.ToLower(got), "ask not what your country can do for you") {
		t.Errorf("unexpected transcript: %q", got)
	}
}
