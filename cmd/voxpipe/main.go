// Command voxpipe is a speech-to-text tool: capture audio, transcribe with whisper.cpp, and
// output to the console, clipboard or a virtual keyboard (ydotool).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"voxpipe/internal/app"
	"voxpipe/internal/audio"
	"voxpipe/internal/config"
	"voxpipe/internal/input"
	"voxpipe/internal/logging"
	"voxpipe/internal/output"
	"voxpipe/internal/text"
	"voxpipe/internal/transcribe"
)

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "", "path to config.toml (default: ./config.toml, then ~/.config/voxpipe/config.toml)")
	flag.Parse()

	log := logging.NewStd()

	cfg, err := config.Load(*configPath, log)
	if err != nil {
		log.Error("Configuration error: " + err.Error())
		return 1
	}

	printStartupInfo(cfg, log)

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()

	modelPath, err := transcribe.NewModelEnsurer(log).Ensure(ctx, cfg.ModelType, cfg.ModelDir)
	if err != nil {
		return fatal(log, ctx, err)
	}

	transformer := text.NewTransformer(cfg.TextTransforms, text.NewReplacer())
	transcriber, err := transcribe.NewTranscriber(modelPath, cfg.Language, cfg.WhisperThreads, transformer, log)
	if err != nil {
		return fatal(log, ctx, err)
	}
	defer transcriber.Close()

	fallback := -1
	if cfg.DeviceIndex != nil {
		fallback = *cfg.DeviceIndex
	}
	resolver := audio.NewResolver(cfg.DeviceName, fallback, log)

	capture, err := audio.NewCapture(resolver, cfg.FrameLength, cfg.CaptureTimeoutSeconds)
	if err != nil {
		return fatal(log, ctx, err)
	}
	defer capture.Close()

	if names, err := capture.DeviceNames(); err != nil {
		log.Warn("Could not list input devices: " + err.Error())
	} else {
		resolver.LogAvailable(names)
	}

	publishers, err := output.Build(cfg.Outputs, os.Stdout, log)
	if err != nil {
		return fatal(log, ctx, err)
	}
	dispatcher, err := output.NewDispatcher(publishers, log)
	if err != nil {
		return fatal(log, ctx, err)
	}

	sources := []input.Source{input.NewConsole()}
	if cfg.HasHTTPSource() {
		sources = append(sources, input.NewHTTP(*cfg.HTTPPort, log))
	}
	cmds, waitSources := input.FanIn(ctx, sources, log)

	timeout := time.Duration(cfg.CaptureTimeoutSeconds) * time.Second
	app.New(capture, transcriber, dispatcher, timeout, log).Run(ctx, cancel, cmds)

	cancel()
	waitSources()
	log.Debug("VoxPipe stopped.")
	return 0
}

// fatal reports err unless it is just a shutdown request.
func fatal(log *logging.Logger, ctx context.Context, err error) int {
	if ctx.Err() != nil {
		return 0
	}
	log.Error("Fatal error:")
	log.ErrorErr(err)
	return 1
}

func printStartupInfo(cfg *config.AppConfig, log *logging.Logger) {
	log.Debug("VoxPipe: a cross platform Speech to Text program")
	log.Debug("Commands:")
	log.Debug("  v = toggle capture (start / stop + transcribe)")
	log.Debug("  c = start capture")
	log.Debug("  z = stop capture and discard audio")
	log.Debug("  x = stop capture and transcribe")
	log.Debug("  q / Esc = quit")

	if !cfg.HasHTTPSource() {
		return
	}
	log.Debug("In another terminal, you can send commands via HTTP:")
	log.Debug(fmt.Sprintf("  curl -s -X POST http://localhost:%d/command/v   # toggle capture", *cfg.HTTPPort))
	log.Debug(fmt.Sprintf("  curl -s -X POST http://localhost:%d/command/q   # quit", *cfg.HTTPPort))
}
