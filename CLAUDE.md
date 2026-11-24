# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

VoxPipe is a Go speech-to-text console application. It captures microphone audio, transcribes it with whisper.cpp (CPU only, via cgo), and outputs to the console, clipboard, or a virtual keyboard (ydotool). It is a port of the earlier C# PvWhisper app (see git history before the Go cutover).

## Build and Test Commands

whisper.cpp is a git submodule built into `build/whisper`. Always go through `make` so cgo finds the static libs; plain `go test` fails to link without the `C_INCLUDE_PATH` and `LIBRARY_PATH` the Makefile exports.

```bash
git submodule update --init --recursive   # first clone only
make build      # builds libwhisper if needed, then bin/voxpipe
make test       # go test ./...
make vet        # go vet ./...
./scripts/run_voxpipe.sh    # run prebuilt bin/voxpipe (requires make build first)
./scripts/toggle_voxpipe.sh # POST /command/v to the HTTP source (default port 5042)

# Single test
make libwhisper && C_INCLUDE_PATH=$PWD/third_party/whisper.cpp/include:$PWD/third_party/whisper.cpp/ggml/include \
  LIBRARY_PATH=$PWD/build/whisper/src:$PWD/build/whisper/ggml/src go test ./internal/text -run TestReplace

# Manual mic check (needs a real audio device)
go run ./tools/rectest -device "C922 Pro" -out /tmp/rec.wav
```

Build prerequisites: Go 1.24+, `gcc`, `g++`, `cmake`, `make`, `libasound2-dev`.

`go.mod` must stay on `go 1.24.5` (the dev container's toolchain). Check a dependency's `go` directive before upgrading it.

## Architecture

```
cmd/voxpipe/main.go     wiring (constructors, signal handling)
tools/rectest/            manual recorder for the audio package
internal/app/             command loop
internal/config/          config.toml load/validate, enums, ~ expansion
internal/logging/         color logger, CAPTURING alert
internal/text/            Replacer (regexp2), Transformer (ordered rules)
internal/audio/           malgo capture, device resolver, WAV, capture timeout
internal/transcribe/      model download, whisper.cpp inference
internal/input/           console + HTTP command sources, fan-in
internal/output/          dispatcher, console/clipboard/ydotool publishers
third_party/whisper.cpp   submodule, pinned to v1.9.6
scripts/                  run, toggle (HTTP) and ydotoold helper scripts
assets/                   logo
.devcontainer/            Containerfile for the Claude dev container (not the app build)
```

**Data flow:** `input.FanIn` merges console keys and HTTP commands into one channel. `app.Run` handles one command at a time (capture state needs no locking there). `audio.Capture` buffers PCM16 16 kHz mono. `transcribe.Transcriber` converts to float32, runs whisper, then applies `text.Transformer`. `output.Dispatcher` publishes to each configured output in order.

**Commands (console key or `POST /command/{c}`):** `v` toggle, `c` start, `x` stop+transcribe, `z` stop+discard, `q`/Esc quit, `i` ignored. Case-insensitive.

## Configuration

`config.toml` is local and gitignored; `config.example.toml` is the checked-in template (`run_voxpipe.sh` copies it on first run). Search order: `--config <path>`, `./config.toml`, `$XDG_CONFIG_HOME/voxpipe/config.toml`. Unknown keys are rejected (go-toml strict mode). Keys:
- `outputs`: `Console`, `Clipboard`, `Ydotool` (case-insensitive)
- `deviceIndex` / `deviceName`: name is a case-insensitive substring match, index is the fallback (-1 = system default)
- `modelType`: `Tiny`, `Base`, `Small`, `Medium`, `Large` (Large downloads large-v3-turbo, saved as `ggml-large.bin`)
- `modelDir`: model directory, `~` expanded, relative paths are relative to the config file, created if missing, default `$XDG_DATA_HOME/voxpipe/models`; models download from HuggingFace if absent
- `httpPort`: enables the HTTP source when > 0
- `captureTimeoutSeconds`: auto-stop after N seconds
- `language`, `whisperThreads`, `frameLength`
- `[[textTransforms]]`: ordered tables of `description, find, replace, isRegex, caseSensitive` (use single-quoted literal strings for regexes); `replace` supports `{N}`, `{N:ToUpper}`, `{N:ToLower}` (regexp2, .NET syntax, 1 s match timeout)

## Notes and gotchas

- Capture timeout discards the audio (matches the C# behavior). `transcribeOnTimeout` in `internal/app/app.go` flips it.
- Passing a segment callback to the whisper.cpp Go bindings forces single-segment mode; `transcribe` reads segments with `NextSegment()` instead.
- Console source puts the terminal in raw mode; Ctrl-C is mapped to quit, escape sequences (arrows) are swallowed.
- Clipboard needs `wl-copy`, `xclip` or `xsel`. Ydotool needs the `ydotoold` socket at `/tmp/.ydotool_socket` (`start_ydotoold.sh`, or `run_voxpipe.sh` prompts to start it under sudo when a tty is attached and reaps it via trap).
- Design: constructor injection, small consumer-side interfaces, no package-level state.
