# Plan: port PvWhisper (C#) to Go

## Decisions (fixed)

1. Audio: `github.com/gen2brain/malgo` (miniaudio, cgo). No Picovoice dependency.
2. Regex: `github.com/dlclark/regexp2` for .NET parity.
3. Replace the C# code in place (delete `src/` C# projects and the .sln in the final step).
4. Whisper: CPU only, whisper.cpp Go bindings (cgo).

Default git workflow: current branch, commit per step, no push.

## Target layout

```
go.mod                        module gowhisper
cmd/gowhisper/main.go         wiring (replaces Program.cs)
internal/app/app.go           command loop (PvWhisperApp.cs)
internal/config/              AppConfig, load/validate, ~ expansion, ModelKind
internal/logging/             Logger (color, ToggleAlert)
internal/text/                RegexReplacer, TextTransformer
internal/audio/               capture (malgo), device resolve, wav, timeout
internal/transcribe/          model ensure + whisper
internal/input/               console, http sources, fan-in
internal/output/              dispatcher, console, clipboard, ydotool
AppConfig.json                unchanged
```

## Behavior to preserve (from reading the C# source)

- Commands (single char, case-insensitive): `v` toggle, `c` start, `x` stop+transcribe, `z` stop+discard, `q` or ESC quit, `i` ignored, anything else logs error.
- Sources: console keys and HTTP `POST /command/{c}` on `localhost:httpPort` (200 on success, 400 otherwise). Enabled when `httpPort > 0`.
- Capture timeout: auto-stop after `captureTimeoutSeconds`, then DISCARD-flag is passed (`HandleStopAndTranscribeAsync(token, true)` returns without transcribing). Port as is; flag to user as probable bug or intent check.
- `CAPTURING` alert toggled around each command.
- Device resolution: `deviceName` case-insensitive substring match, fallback to `deviceIndex` (default -1 = system default), logs on index change.
- Audio: PCM16 mono 16 kHz, frame size `frameLength` (default 512), wrapped to 44-byte-header WAV before whisper.
- Transforms: ordered list of `{find, replace, isRegex, caseSensitive, description}`. Replace template tokens `{N}`, `{N:ToUpper}`, `{N:ToLower}`; out-of-range N yields empty.
- Ydotool: leading `{ydotool:KEY_BACKSPACE}` runs `ydotool key 14:1 14:0`, remainder typed via `ydotool type --key-delay=3 --key-hold=2 --file=-`, env `YDOTOOL_SOCKET=/tmp/.ydotool_socket`. Empty text skipped with warning.
- Clipboard: skip empty, log failures as warnings.
- Model: `modelType` Tiny/Base/Small/Medium/Large, `modelDir` with `~` expansion, auto-download if missing (HuggingFace GGML).
- Config keys: outputs (case-insensitive, `ydotool` lowercase appears in AppConfig.json), deviceIndex, deviceName, language, modelDir, modelType, httpPort, whisperThreads, frameLength, captureTimeoutSeconds, textTransforms.

## Steps

### 1. Scaffold + pure logic (no cgo)
- `go mod init`, layout above.
- `internal/config`: structs with `json` tags, case-insensitive enum decode, defaults matching `AppConfig.cs`, validation per `ConfigService.cs` (read it first, port its checks).
- `internal/text`: `Replacer` using regexp2, cache keyed on (pattern, options) with 64 cap, token expansion. Options: `IgnoreCase` when `!caseSensitive`; non-regex rules use `regexp2.Escape`. Use `strings.ToUpper/ToLower` (note: .NET used CurrentCulture, Go is invariant; acceptable).
- `internal/text`: `Transformer` applying rules in order (port `TextTransformer.cs`).
- `internal/logging`: port `Logger.cs` (levels, colors, `ToggleAlert`).
- Port `RegexReplacerTests.cs` as table tests. Commit.

### 2. Audio
- `wav.go`: 44-byte header writer via `encoding/binary`; port `WavConverterTests.cs`.
- `device.go`: enumerate with malgo capture devices, resolution logic above. Interface over the enumerator so it is unit-testable.
- `capture.go`: malgo capture device at 16 kHz mono S16, data callback appends to a mutex-guarded buffer (preallocate as in latest commit "size audio buffers"). Methods: `Start`, `StopAndGet`, `StopAndDiscard`, `IsCapturing`. Note malgo delivers arbitrary-size callbacks, so `frameLength` maps to `PeriodSizeInFrames`.
- `timeout.go`: `context.WithTimeout` per capture, restart on start, cancel on stop; callback runs on expiry.
- Verify manually: record 3 s, write WAV, play back. Commit.

### 3. Transcription (cgo)
- Pin whisper.cpp (git submodule under `third_party/whisper.cpp` or fetch script). Module path is `github.com/ggerganov/whisper.cpp/bindings/go` (verify current path/org before pinning).
- `Makefile`: build libwhisper (CPU only), export `CGO_CFLAGS`, `CGO_LDFLAGS`, `LIBRARY_PATH`; targets `build`, `test`, `run`.
- `model.go`: ensure model file exists, download with `net/http` to temp file then rename, progress logging.
- `whisper.go`: load model once, per-call context with `language`, `threads`, collect segment text, trim, run transforms. Convert samples to float32 directly (skip WAV round trip; the Go bindings take `[]float32`). Keep WAV writer for tests and debug dump only.
- Smoke test on a known sample WAV. Commit.

### 4. Output
- `Publisher` interface: `Publish(ctx, text) error`.
- Console: print. Clipboard: try `wl-copy`, then `xclip -selection clipboard`, then `xsel`; warn if none (replaces TextCopy).
- Ydotool: `os/exec` directly (no `bash -c`), env var, stdin piping, exit code + stderr warning.
- Dispatcher: fan out to configured outputs sequentially, log per-publisher failures. Commit.

### 5. Input + app loop
- `Source` interface: `Run(ctx, out chan<- rune)`.
- Console: if stdin is a TTY use `golang.org/x/term` raw mode for single keypress, else read bytes. Restore terminal on exit.
- HTTP: `net/http` server on `localhost:port`, `POST /command/{c}`, graceful shutdown with 2 s deadline.
- Fan-in to one buffered channel; `app.Run` select loop on channel and `ctx.Done()`; `signal.NotifyContext` for SIGINT/SIGTERM.
- `main.go`: load config path (`AppConfig.json` next to binary or cwd), wire dependencies by constructors. Commit.

### 6. Cutover
- Update `run_pvw.sh`, `toggle_pvw.sh`, `start_ydotoold.sh` references (build/run commands; toggle likely `curl -X POST localhost:5042/command/v`).
- Delete `src/` (C# projects, tests, sln).
- Rewrite `CLAUDE.md` (stale: documents a named pipe and `pipePath`, code actually uses HTTP) and `README.md` for Go: build prerequisites (`gcc`, `g++`, `cmake`, `make`, `libasound2-dev`), commands, config.
- `Containerfile` is the Claude dev container, not the app; add Go toolchain + build deps there only if the user wants to build inside it (ask).
- Run `go vet ./...`, `go test ./...`, report failures. Final commit.

## Risks

- cgo + whisper.cpp build is the long pole; prove step 3 smoke test before investing in the rest of steps 4 and 5.
- regexp2 is slower than RE2 and can backtrack catastrophically on bad user patterns; set `MatchTimeout` (e.g. 1 s).
- malgo needs ALSA or PulseAudio dev headers at build time; runtime device names may differ from PvRecorder's, so `deviceName` ("C922 Pro") substring may need rechecking.
- Transcript output may differ slightly from Whisper.net (different whisper.cpp version, default decode params). Compare on a sample before cutover.
- Push is never done automatically; remind after commits.
