// Package audio handles microphone capture, device resolution and WAV encoding.
package audio

import "encoding/binary"

// WavHeaderBytes is RIFF chunk (12) + fmt subchunk (24) + data subchunk header (8).
const WavHeaderBytes = 44

// WavFromPCM16 wraps mono 16-bit PCM samples in a canonical 44-byte-header WAV.
func WavFromPCM16(samples []int16, sampleRate int) []byte {
	const (
		bytesPerSample = 2
		numChannels    = 1
		bitsPerSample  = 16
	)
	dataSize := len(samples) * bytesPerSample

	// Exact capacity, so the slice never has to grow and copy itself
	buf := make([]byte, WavHeaderBytes+dataSize)
	le := binary.LittleEndian

	copy(buf[0:], "RIFF")
	le.PutUint32(buf[4:], uint32(4+(8+16)+(8+dataSize)))
	copy(buf[8:], "WAVE")

	copy(buf[12:], "fmt ")
	le.PutUint32(buf[16:], 16)
	le.PutUint16(buf[20:], 1) // PCM
	le.PutUint16(buf[22:], numChannels)
	le.PutUint32(buf[24:], uint32(sampleRate))
	le.PutUint32(buf[28:], uint32(sampleRate*numChannels*bytesPerSample))
	le.PutUint16(buf[32:], numChannels*bytesPerSample)
	le.PutUint16(buf[34:], bitsPerSample)

	copy(buf[36:], "data")
	le.PutUint32(buf[40:], uint32(dataSize))

	for i, s := range samples {
		le.PutUint16(buf[WavHeaderBytes+i*2:], uint16(s))
	}
	return buf
}
