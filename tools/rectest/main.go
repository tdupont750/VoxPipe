// Command rectest records a few seconds from the configured device and writes a WAV file.
// Manual check for the audio package: go run ./tools/rectest -out /tmp/rec.wav
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"voxpipe/internal/audio"
	"voxpipe/internal/logging"
)

func main() {
	out := flag.String("out", "rec.wav", "output WAV path")
	secs := flag.Int("seconds", 3, "seconds to record")
	name := flag.String("device", "", "device name substring")
	index := flag.Int("index", -1, "device index fallback")
	flag.Parse()

	log := logging.NewStd()
	cap, err := audio.NewCapture(audio.NewResolver(*name, *index, log), 512, *secs+5)
	if err != nil {
		fatal(err)
	}
	defer cap.Close()

	names, err := cap.DeviceNames()
	if err != nil {
		fatal(err)
	}
	audio.NewResolver(*name, *index, log).LogAvailable(names)

	if err := cap.Start(); err != nil {
		fatal(err)
	}
	time.Sleep(time.Duration(*secs) * time.Second)

	samples, err := cap.StopAndGet()
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, audio.WavFromPCM16(samples, audio.SampleRate), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d samples)\n", *out, len(samples))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
